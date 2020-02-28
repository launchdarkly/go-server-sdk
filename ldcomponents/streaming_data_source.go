package ldcomponents

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	es "github.com/launchdarkly/eventsource"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldevents"
)

const (
	putEvent           = "put"
	patchEvent         = "patch"
	deleteEvent        = "delete"
	indirectPatchEvent = "indirect/patch"
	streamReadTimeout  = 5 * time.Minute // the LaunchDarkly stream should send a heartbeat comment every 3 minutes
)

type streamProcessor struct {
	store                      interfaces.DataStore
	streamURI                  string
	initialReconnectDelay      time.Duration
	client                     *http.Client
	requestor                  *requestor
	headers                    http.Header
	diagnosticsManager         *ldevents.DiagnosticsManager
	loggers                    ldlog.Loggers
	setInitializedOnce         sync.Once
	isInitialized              bool
	halt                       chan struct{}
	storeStatusSub             internal.DataStoreStatusSubscription
	connectionAttemptStartTime ldtime.UnixMillisecondTime
	readyOnce                  sync.Once
	closeOnce                  sync.Once
}

type putData struct {
	Path string  `json:"path"`
	Data allData `json:"data"`
}

type patchData struct {
	Path string `json:"path"`
	// This could be a flag or a segment, or something else, depending on the path
	Data json.RawMessage `json:"data"`
}

type deleteData struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
}

// This interface is implemented only by the SDK's own ClientContext implementation.
type hasDiagnosticsManager interface {
	GetDiagnosticsManager() *ldevents.DiagnosticsManager
}

func (sp *streamProcessor) Initialized() bool {
	return sp.isInitialized
}

func (sp *streamProcessor) Start(closeWhenReady chan<- struct{}) {
	sp.loggers.Info("Starting LaunchDarkly streaming connection")
	if fss, ok := sp.store.(internal.DataStoreStatusProvider); ok {
		sp.storeStatusSub = fss.StatusSubscribe()
	}
	go sp.subscribe(closeWhenReady)
}

type parsedPath struct {
	key  string
	kind interfaces.VersionedDataKind
}

func parsePath(path string) (parsedPath, error) {
	parsedPath := parsedPath{}
	if strings.HasPrefix(path, "/segments/") {
		parsedPath.kind = interfaces.DataKindSegments()
		parsedPath.key = strings.TrimPrefix(path, "/segments/")
	} else if strings.HasPrefix(path, "/flags/") {
		parsedPath.kind = interfaces.DataKindFeatures()
		parsedPath.key = strings.TrimPrefix(path, "/flags/")
	} else {
		return parsedPath, fmt.Errorf("unrecognized path %s", path)
	}
	return parsedPath, nil
}

// Returns true if we should recreate the stream and start over
func (sp *streamProcessor) events(stream *es.Stream, closeWhenReady chan<- struct{}) bool {
	notifyReady := func() {
		sp.readyOnce.Do(func() {
			close(closeWhenReady)
		})
	}
	// Ensure we stop waiting for initialization if we exit, even if initialization fails
	defer notifyReady()

	// Consume remaining Events and Errors so we can garbage collect
	defer func() {
		for range stream.Events {
		}
		for range stream.Errors {
		}
	}()

	var statusCh <-chan internal.DataStoreStatus
	if sp.storeStatusSub != nil {
		statusCh = sp.storeStatusSub.Channel()
	}

	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				sp.loggers.Info("Event stream closed")
				return false
			}
			sp.logConnectionResult(true)
			switch event.Event() {
			case putEvent:
				var put putData
				if err := json.Unmarshal([]byte(event.Data()), &put); err != nil {
					sp.loggers.Errorf("Unexpected error unmarshalling PUT json: %+v", err)
					break
				}
				err := sp.store.Init(makeAllVersionedDataMap(put.Data.Flags, put.Data.Segments))
				if err != nil {
					sp.loggers.Errorf("Error initializing store: %s", err)
					return false
				}
				sp.setInitializedOnce.Do(func() {
					sp.loggers.Info("LaunchDarkly streaming is active")
					sp.isInitialized = true
					notifyReady()
				})
			case patchEvent:
				var patch patchData
				if err := json.Unmarshal([]byte(event.Data()), &patch); err != nil {
					sp.loggers.Errorf("Unexpected error unmarshalling PATCH json: %+v", err)
					break
				}
				path, err := parsePath(patch.Path)
				if err != nil {
					sp.loggers.Errorf("Unable to process event %s: %s", event.Event(), err)
					break
				}
				item := path.kind.GetDefaultItem().(interfaces.VersionedData)
				if err = json.Unmarshal(patch.Data, item); err != nil {
					sp.loggers.Errorf("Unexpected error unmarshalling JSON for %s item: %+v", path.kind, err)
					break
				}
				if err = sp.store.Upsert(path.kind, item); err != nil {
					sp.loggers.Errorf("Unexpected error storing %s item: %+v", path.kind, err)
				}
			case deleteEvent:
				var data deleteData
				if err := json.Unmarshal([]byte(event.Data()), &data); err != nil {
					sp.loggers.Errorf("Unexpected error unmarshalling DELETE json: %+v", err)
					break
				}
				path, err := parsePath(data.Path)
				if err != nil {
					sp.loggers.Errorf("Unable to process event %s: %s", event.Event(), err)
					break
				}
				if err = sp.store.Delete(path.kind, path.key, data.Version); err != nil {
					sp.loggers.Errorf(`Unexpected error deleting %s item "%s": %s`, path.kind, path.key, err)
				}
			case indirectPatchEvent:
				path, err := parsePath(event.Data())
				if err != nil {
					sp.loggers.Errorf("Unable to process event %s: %s", event.Event(), err)
					break
				}
				item, requestErr := sp.requestor.requestResource(path.kind, path.key)
				if requestErr != nil {
					sp.loggers.Errorf(`Unexpected error requesting %s item "%s": %+v`, path.kind, path.key, err)
					break
				}
				if err = sp.store.Upsert(path.kind, item); err != nil {
					sp.loggers.Errorf(`Unexpected error store %s item "%s": %+v`, path.kind, path.key, err)
				}
			default:
				sp.loggers.Infof("Unexpected event found in stream: %s", event.Event())
			}
		case err, ok := <-stream.Errors:
			if !ok {
				sp.loggers.Info("Event error stream closed")
				return false // Otherwise we will spin in this loop
			}
			sp.loggers.Error(err)
			if err != io.EOF {
				sp.loggers.Errorf("Error encountered processing stream: %+v", err)
				if sp.checkIfPermanentFailure(err) {
					sp.closeOnce.Do(func() {
						sp.loggers.Info("Closing event stream")
						stream.Close()
					})
					return false
				}
			}
		case newStoreStatus := <-statusCh:
			if newStoreStatus.Available && newStoreStatus.NeedsRefresh {
				// The store has just transitioned from unavailable to available, and we can't guarantee that
				// all of the latest data got cached, so let's restart the stream to refresh all the data.
				sp.loggers.Warn("Restarting stream to refresh data after data store outage")
				stream.Close()
				return true // causes subscribe() to restart the connection
			}
		case <-sp.halt:
			stream.Close()
			return false
		}
	}
}

func newStreamProcessor(
	context interfaces.ClientContext,
	store interfaces.DataStore,
	streamURI string,
	initialReconnectDelay time.Duration,
	requestor *requestor,
) *streamProcessor {
	sp := &streamProcessor{
		store:                 store,
		streamURI:             streamURI,
		initialReconnectDelay: initialReconnectDelay,
		requestor:             requestor,
		headers:               context.GetDefaultHTTPHeaders(),
		loggers:               context.GetLoggers(),
		halt:                  make(chan struct{}),
	}
	if hdm, ok := context.(hasDiagnosticsManager); ok {
		sp.diagnosticsManager = hdm.GetDiagnosticsManager()
	}

	sp.client = context.CreateHTTPClient()
	// Client.Timeout isn't just a connect timeout, it will break the connection if a full response
	// isn't received within that time (which, with the stream, it never will be), so we must make
	// sure it's zero and not the usual configured default. What we do want is a *connection* timeout,
	// which is set by Config.newHTTPClient as a property of the Dialer.
	sp.client.Timeout = 0

	return sp
}

func (sp *streamProcessor) subscribe(closeWhenReady chan<- struct{}) {
	for {
		req, _ := http.NewRequest("GET", sp.streamURI+"/all", nil)
		for k, vv := range sp.headers {
			req.Header[k] = vv
		}
		sp.loggers.Info("Connecting to LaunchDarkly stream")

		sp.logConnectionStarted()

		if stream, err := es.SubscribeWithRequestAndOptions(req,
			es.StreamOptionHTTPClient(sp.client),
			es.StreamOptionReadTimeout(streamReadTimeout),
			es.StreamOptionInitialRetry(sp.initialReconnectDelay),
			es.StreamOptionLogger(sp.loggers.ForLevel(ldlog.Info))); err != nil {

			sp.loggers.Warnf("Unable to establish streaming connection: %+v", err)
			sp.logConnectionResult(false)

			if sp.checkIfPermanentFailure(err) {
				close(closeWhenReady)
				return
			}

			// Halt immediately if we've been closed already
			select {
			case <-sp.halt:
				close(closeWhenReady)
				return
			default:
				time.Sleep(2 * time.Second)
			}
		} else {
			if !sp.events(stream, closeWhenReady) {
				return
			}
			// If events() returned true, we should continue the for loop
		}
	}
}

func (sp *streamProcessor) checkIfPermanentFailure(err error) bool {
	if se, ok := err.(es.SubscriptionError); ok {
		sp.loggers.Error(httpErrorMessage(se.Code, "streaming connection", "will retry"))
		if !isHTTPErrorRecoverable(se.Code) {
			return true
		}
	}
	return false
}

func (sp *streamProcessor) logConnectionStarted() {
	sp.connectionAttemptStartTime = ldtime.UnixMillisNow()
}

func (sp *streamProcessor) logConnectionResult(success bool) {
	if sp.connectionAttemptStartTime > 0 && sp.diagnosticsManager != nil {
		timestamp := ldtime.UnixMillisNow()
		sp.diagnosticsManager.RecordStreamInit(timestamp, !success,
			uint64(timestamp-sp.connectionAttemptStartTime))
	}
	sp.connectionAttemptStartTime = 0
}

// Close instructs the processor to stop receiving updates
func (sp *streamProcessor) Close() error {
	sp.closeOnce.Do(func() {
		sp.loggers.Info("Closing event stream")
		close(sp.halt)
		if sp.storeStatusSub != nil {
			sp.storeStatusSub.Close()
		}
	})
	return nil
}

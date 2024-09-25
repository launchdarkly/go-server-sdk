package datasourcev2

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	es "github.com/launchdarkly/eventsource"

	"golang.org/x/exp/maps"
)

const (
	keyField     = "key"
	kindField    = "kind"
	versionField = "version"

	streamReadTimeout        = 5 * time.Minute // the LaunchDarkly stream should send a heartbeat comment every 3 minutes
	streamMaxRetryDelay      = 30 * time.Second
	streamRetryResetInterval = 60 * time.Second
	streamJitterRatio        = 0.5
	defaultStreamRetryDelay  = 1 * time.Second

	streamingErrorContext     = "in stream connection"
	streamingWillRetryMessage = "will retry"
)

// Implementation of the streaming data source, not including the lower-level SSE implementation which is in
// the eventsource package.
//
// Error handling works as follows:
// 1. If any event is malformed, we must assume the stream is broken and we may have missed updates. Set the
// data source state to INTERRUPTED, with an error kind of INVALID_DATA, and restart the stream.
// 2. If we try to put updates into the data store and we get an error, we must assume something's wrong with the
// data store. We don't have to log this error because it is logged by DataSourceUpdateSinkImpl, which will also set
// our state to INTERRUPTED for us.
// 2a. If the data store supports status notifications (which all persistent stores normally do), then we can
// assume it has entered a failed state and will notify us once it is working again. If and when it recovers, then
// it will tell us whether we need to restart the stream (to ensure that we haven't missed any updates), or
// whether it has already persisted all of the stream updates we received during the outage.
// 2b. If the data store doesn't support status notifications (which is normally only true of the in-memory store)
// then we don't know the significance of the error, but we must assume that updates have been lost, so we'll
// restart the stream.
// 3. If we receive an unrecoverable error like HTTP 401, we close the stream and don't retry, and set the state
// to OFF. Any other HTTP error or network error causes a retry with backoff, with a state of INTERRUPTED.
// 4. We set the Future returned by start() to tell the client initialization logic that initialization has either
// succeeded (we got an initial payload and successfully stored it) or permanently failed (we got a 401, etc.).
// Otherwise, the client initialization method may time out but we will still be retrying in the background, and
// if we succeed then the client can detect that we're initialized now by calling our Initialized method.

// StreamProcessor is the internal implementation of the streaming data source.
//
// This type is exported from internal so that the StreamingDataSourceBuilder tests can verify its
// configuration. All other code outside of this package should interact with it only via the
// DataSource interface.
type StreamProcessor struct {
	cfg                        datasource.StreamConfig
	dataDestination            subsystems.DataDestination
	statusReporter             subsystems.DataSourceStatusReporter
	client                     *http.Client
	headers                    http.Header
	diagnosticsManager         *ldevents.DiagnosticsManager
	loggers                    ldlog.Loggers
	isInitialized              internal.AtomicBoolean
	halt                       chan struct{}
	connectionAttemptStartTime ldtime.UnixMillisecondTime
	connectionAttemptLock      sync.Mutex
	readyOnce                  sync.Once
	closeOnce                  sync.Once
}

// NewStreamProcessor creates the internal implementation of the streaming data source.
func NewStreamProcessor(
	context subsystems.ClientContext,
	dataDestination subsystems.DataDestination,
	statusReporter subsystems.DataSourceStatusReporter,
	cfg datasource.StreamConfig,
) *StreamProcessor {
	sp := &StreamProcessor{
		dataDestination: dataDestination,
		statusReporter:  statusReporter,
		headers:         context.GetHTTP().DefaultHeaders,
		loggers:         context.GetLogging().Loggers,
		halt:            make(chan struct{}),
		cfg:             cfg,
	}
	if cci, ok := context.(*internal.ClientContextImpl); ok {
		sp.diagnosticsManager = cci.DiagnosticsManager
	}

	sp.client = context.GetHTTP().CreateHTTPClient()
	// Client.Timeout isn't just a connect timeout, it will break the connection if a full response
	// isn't received within that time (which, with the stream, it never will be), so we must make
	// sure it's zero and not the usual configured default. What we do want is a *connection* timeout,
	// which is set by Config.newHTTPClient as a property of the Dialer.
	sp.client.Timeout = 0

	return sp
}

//nolint:revive // no doc comment for standard method
func (sp *StreamProcessor) IsInitialized() bool {
	return sp.isInitialized.Get()
}

//nolint:revive // no doc comment for standard method
func (sp *StreamProcessor) Start(closeWhenReady chan<- struct{}) {
	sp.loggers.Info("Starting LaunchDarkly streaming connection")
	go sp.subscribe(closeWhenReady)
}

// TODO(SDK-713): Refactor the implementation of this function to lower the complexity.
//
//nolint:godox
func (sp *StreamProcessor) consumeStream(stream *es.Stream, closeWhenReady chan<- struct{}) {
	// Consume remaining Events and Errors so we can garbage collect
	defer func() {
		for range stream.Events {
		} // COVERAGE: no way to cause this condition in unit tests
		if stream.Errors != nil {
			for range stream.Errors { // COVERAGE: no way to cause this condition in unit tests
			}
		}
	}()

	currentChangeSet := changeSet{
		events: make([]es.Event, 0),
	}

	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				// COVERAGE: stream.Events is only closed if the EventSource has been closed. However, that
				// only happens when we have received from sp.halt, in which case we return immediately
				// after calling stream.Close(), terminating the for loop-- so we should not actually reach
				// this point. Still, in case the channel is somehow closed unexpectedly, we do want to
				// terminate the loop.
				return
			}

			sp.logConnectionResult(true)

			processedEvent := true
			shouldRestart := false

			gotMalformedEvent := func(event es.Event, err error) {
				if event == nil {
					sp.loggers.Errorf(
						"Received streaming events with malformed JSON data (%s); will restart stream",
						err,
					)
				} else {
					sp.loggers.Errorf(
						"Received streaming \"%s\" event with malformed JSON data (%s); will restart stream",
						event.Event(),
						err,
					)
				}

				errorInfo := interfaces.DataSourceErrorInfo{
					Kind:    interfaces.DataSourceErrorKindInvalidData,
					Message: err.Error(),
					Time:    time.Now(),
				}
				sp.statusReporter.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)

				shouldRestart = true // scenario 1 in error handling comments at top of file
				processedEvent = false
			}

			switch fdv2proto.EventName(event.Event()) {
			case fdv2proto.EventHeartbeat:
				// Swallow the event and move on.
			case fdv2proto.EventServerIntent:
				//nolint: godox
				// TODO: Replace all this json unmarshalling with a nicer jreader implementation.
				var serverIntent serverIntent
				err := json.Unmarshal([]byte(event.Data()), &serverIntent)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				} else if len(serverIntent.Payloads) == 0 {
					gotMalformedEvent(event, errors.New("server-intent event has no payloads"))
					break
				}

				if serverIntent.Payloads[0].Code == "none" {
					sp.loggers.Info("Server intent is none, skipping")
					continue
				}

				currentChangeSet = changeSet{events: make([]es.Event, 0), intent: &serverIntent}

			case fdv2proto.EventPutObject:
				currentChangeSet.events = append(currentChangeSet.events, event)
			case fdv2proto.EventDeleteObject:
				currentChangeSet.events = append(currentChangeSet.events, event)
			case fdv2proto.EventGoodbye:
				var goodbye goodbye
				err := json.Unmarshal([]byte(event.Data()), &goodbye)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}

				if !goodbye.Silent {
					sp.loggers.Errorf("SSE server received error: %s (%s)", goodbye.Reason, goodbye.Catastrophe)
				}
			case fdv2proto.EventError:
				var errorData errorEvent
				err := json.Unmarshal([]byte(event.Data()), &errorData)
				if err != nil {
					//nolint: godox
					// TODO: Confirm that an error means we have to discard
					// everything that has come before.
					currentChangeSet = changeSet{events: make([]es.Event, 0)}
					gotMalformedEvent(event, err)
					break
				}

				sp.loggers.Errorf("Error on %s: %s", errorData.PayloadID, errorData.Reason)

				currentChangeSet = changeSet{events: make([]es.Event, 0)}
				//nolint: godox
				// TODO: Do we need to restart here?
			case fdv2proto.EventPayloadTransferred:
				currentChangeSet.events = append(currentChangeSet.events, event)
				updates, err := deserializeEvents(currentChangeSet.events)
				if err != nil {
					sp.loggers.Errorf("Error processing changeset: %s", err)
					gotMalformedEvent(nil, err)
					break
				}

				//nolint:godox
				// TODO(SDK-712): If the destination fails to apply the updates should it affect the processor?
				// This would only happen in the case of a persistent store.

				switch currentChangeSet.intent.Payloads[0].Code {
				case fdv2proto.IntentTransferFull:
					{
						_ = sp.dataDestination.SetBasis(updates, fdv2proto.NoSelector(), true)
						sp.setInitializedAndNotifyClient(true, closeWhenReady)
					}
				case fdv2proto.IntentTransferChanges:
					_ = sp.dataDestination.ApplyDelta(updates, fdv2proto.NoSelector(), true)
				}

				currentChangeSet = changeSet{events: make([]es.Event, 0)}
			default:
				sp.loggers.Infof("Unexpected event found in stream: %s", event.Event())
			}

			if processedEvent {
				sp.statusReporter.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
			}
			if shouldRestart {
				stream.Restart()
			}

		case <-sp.halt:
			stream.Close()
			return
		}
	}
}

func (sp *StreamProcessor) subscribe(closeWhenReady chan<- struct{}) {
	req, reqErr := http.NewRequest("GET", endpoints.AddPath(sp.cfg.URI, endpoints.StreamingRequestPath), nil)
	if reqErr != nil {
		sp.loggers.Errorf(
			"Unable to create a stream request; this is not a network problem, most likely a bad base URI: %s",
			reqErr,
		)
		sp.statusReporter.UpdateStatus(interfaces.DataSourceStateOff, interfaces.DataSourceErrorInfo{
			Kind:    interfaces.DataSourceErrorKindUnknown,
			Message: reqErr.Error(),
			Time:    time.Now(),
		})
		sp.logConnectionResult(false)
		close(closeWhenReady)
		return
	}
	if sp.cfg.FilterKey != "" {
		req.URL.RawQuery = url.Values{
			"filter": {sp.cfg.FilterKey},
		}.Encode()
	}
	if sp.headers != nil {
		req.Header = maps.Clone(sp.headers)
	}
	sp.loggers.Info("Connecting to LaunchDarkly stream")

	sp.logConnectionStarted()

	initialRetryDelay := sp.cfg.InitialReconnectDelay
	if initialRetryDelay <= 0 { // COVERAGE: can't cause this condition in unit tests
		initialRetryDelay = defaultStreamRetryDelay
	}

	errorHandler := func(err error) es.StreamErrorHandlerResult {
		sp.logConnectionResult(false)

		if se, ok := err.(es.SubscriptionError); ok {
			errorInfo := interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindErrorResponse,
				StatusCode: se.Code,
				Time:       time.Now(),
			}
			recoverable := checkIfErrorIsRecoverableAndLog(
				sp.loggers,
				httpErrorDescription(se.Code),
				streamingErrorContext,
				se.Code,
				streamingWillRetryMessage,
			)
			if recoverable {
				sp.logConnectionStarted()
				sp.statusReporter.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)
				return es.StreamErrorHandlerResult{CloseNow: false}
			}
			sp.statusReporter.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)
			return es.StreamErrorHandlerResult{CloseNow: true}
		}

		checkIfErrorIsRecoverableAndLog(
			sp.loggers,
			err.Error(),
			streamingErrorContext,
			0,
			streamingWillRetryMessage,
		)
		errorInfo := interfaces.DataSourceErrorInfo{
			Kind:    interfaces.DataSourceErrorKindNetworkError,
			Message: err.Error(),
			Time:    time.Now(),
		}
		sp.statusReporter.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)
		sp.logConnectionStarted()
		return es.StreamErrorHandlerResult{CloseNow: false}
	}

	stream, err := es.SubscribeWithRequestAndOptions(req,
		es.StreamOptionHTTPClient(sp.client),
		es.StreamOptionReadTimeout(streamReadTimeout),
		es.StreamOptionInitialRetry(initialRetryDelay),
		es.StreamOptionUseBackoff(streamMaxRetryDelay),
		es.StreamOptionUseJitter(streamJitterRatio),
		es.StreamOptionRetryResetInterval(streamRetryResetInterval),
		es.StreamOptionErrorHandler(errorHandler),
		es.StreamOptionCanRetryFirstConnection(-1),
		es.StreamOptionLogger(sp.loggers.ForLevel(ldlog.Info)),
	)

	if err != nil {
		sp.logConnectionResult(false)

		close(closeWhenReady)
		return
	}

	sp.consumeStream(stream, closeWhenReady)
}

func (sp *StreamProcessor) setInitializedAndNotifyClient(success bool, closeWhenReady chan<- struct{}) {
	if success {
		wasAlreadyInitialized := sp.isInitialized.GetAndSet(true)
		if !wasAlreadyInitialized {
			sp.loggers.Info("LaunchDarkly streaming is active")
		}
	}
	sp.readyOnce.Do(func() {
		close(closeWhenReady)
	})
}

func (sp *StreamProcessor) logConnectionStarted() {
	sp.connectionAttemptLock.Lock()
	defer sp.connectionAttemptLock.Unlock()
	sp.connectionAttemptStartTime = ldtime.UnixMillisNow()
}

func (sp *StreamProcessor) logConnectionResult(success bool) {
	sp.connectionAttemptLock.Lock()
	startTimeWas := sp.connectionAttemptStartTime
	sp.connectionAttemptStartTime = 0
	sp.connectionAttemptLock.Unlock()

	if startTimeWas > 0 && sp.diagnosticsManager != nil {
		timestamp := ldtime.UnixMillisNow()
		sp.diagnosticsManager.RecordStreamInit(timestamp, !success, uint64(timestamp-startTimeWas))
	}
}

//nolint:revive // no doc comment for standard method
func (sp *StreamProcessor) Close() error {
	sp.closeOnce.Do(func() {
		close(sp.halt)
		sp.statusReporter.UpdateStatus(interfaces.DataSourceStateOff, interfaces.DataSourceErrorInfo{})
	})
	return nil
}

// GetBaseURI returns the configured streaming base URI, for testing.
func (sp *StreamProcessor) GetBaseURI() string {
	return sp.cfg.URI
}

// GetInitialReconnectDelay returns the configured reconnect delay, for testing.
func (sp *StreamProcessor) GetInitialReconnectDelay() time.Duration {
	return sp.cfg.InitialReconnectDelay
}

// GetFilterKey returns the configured key, for testing.
func (sp *StreamProcessor) GetFilterKey() string {
	return sp.cfg.FilterKey
}

func deserializeEvents(events []es.Event) ([]fdv2proto.Event, error) {
	updates := make([]fdv2proto.Event, 0, len(events))

	parseItem := func(r jreader.Reader, kind datakinds.DataKindInternal) (ldstoretypes.ItemDescriptor, error) {
		item, err := kind.DeserializeFromJSONReader(&r)
		return item, err
	}

	for _, event := range events {
		switch fdv2proto.EventName(event.Event()) {
		case fdv2proto.EventPutObject:
			r := jreader.NewReader([]byte(event.Data()))
			// var version int
			var dataKind datakinds.DataKindInternal
			var key string
			var item ldstoretypes.ItemDescriptor
			var err error

			for obj := r.Object().WithRequiredProperties([]string{versionField, kindField, keyField, "object"}); obj.Next(); {
				switch string(obj.Name()) {
				case versionField:
					// version = r.Int()
				case kindField:
					kind := r.String()
					dataKind = dataKindFromKind(kind)
					if dataKind == nil {
						//nolint: godox
						// TODO: We are skipping here without showing a warning. Need to address that later.
						continue
					}
				case keyField:
					key = r.String()
				case "object":
					item, err = parseItem(r, dataKind)
					if err != nil {
						return updates, err
					}
				}
			}
			updates = append(updates, fdv2proto.PutObject{Kind: dataKind, Key: key, Object: item})
		case fdv2proto.EventDeleteObject:
			r := jreader.NewReader([]byte(event.Data()))
			var version int
			var dataKind datakinds.DataKindInternal
			var kind, key string

			for obj := r.Object().WithRequiredProperties([]string{versionField, kindField, keyField}); obj.Next(); {
				switch string(obj.Name()) {
				case versionField:
					version = r.Int()
				case kindField:
					kind = strings.TrimRight(r.String(), "s")
					dataKind = dataKindFromKind(kind)
					if dataKind == nil {
						//nolint: godox
						// TODO: We are skipping here without showing a warning. Need to address that later.
						continue
					}
				case keyField:
					key = r.String()
				}
			}
			updates = append(updates, fdv2proto.DeleteObject{Kind: dataKind, Key: key, Version: version})
		}
	}

	return updates, nil
}

func dataKindFromKind(kind string) datakinds.DataKindInternal {
	switch kind {
	case "flag":
		return datakinds.Features
	case "segment":
		return datakinds.Segments
	default:
		return nil
	}
}

// vim: foldmethod=marker foldlevel=0

package datasource

import (
	gocontext "context"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	es "github.com/launchdarkly/eventsource"

	"maps"
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
// 3. If we receive an HTTP or transport-level error, we classify it and continue retrying:
// normal errors (400, 408, 429, 5xx, most I/O errors) use the default backoff profile; unexpected errors
// (401, 403, other 4xx, TLS/cert failures) engage the extended-regime profile.
// No HTTP status is terminal and in every case we set state to INTERRUPTED.
// 4. We set the Future returned by start() to tell the client initialization logic that initialization has either
// succeeded (we got an initial payload and successfully stored it) or permanently failed (unparseable base URI).
// Otherwise, the client initialization method may time out but we will still be retrying in the background, and
// if we succeed then the client can detect that we're initialized now by calling our Initialized method.

const (
	putEvent    = "put"
	patchEvent  = "patch"
	deleteEvent = "delete"
	// The LaunchDarkly stream should send a heartbeat comment every 3 minutes.
	streamReadTimeout               = 5 * time.Minute
	streamJitterRatio               = 0.5
	streamRetryResetInterval        = 60 * time.Second
	defaultStreamRetryDelay         = 1 * time.Second
	streamMaxRetryDelay             = 30 * time.Second
	defaultStreamExtendedRetryDelay = 5 * time.Minute
	streamExtendedMaxRetryDelay     = 1 * time.Hour

	streamingErrorContext     = "in stream connection"
	streamingWillRetryMessage = "will retry"
)

// StreamConfig describes the configuration for a streaming data source. It is exported so that
// it can be used in the StreamingDataSourceBuilder.
type StreamConfig struct {
	URI                           string
	FilterKey                     string
	InitialReconnectDelay         time.Duration
	ExtendedInitialReconnectDelay time.Duration
	RetryResetInterval            time.Duration
}

// StreamProcessor is the internal implementation of the streaming data source.
//
// This type is exported from internal so that the StreamingDataSourceBuilder tests can verify its
// configuration. All other code outside of this package should interact with it only via the
// DataSource interface.
type StreamProcessor struct {
	cfg                        StreamConfig
	dataSourceUpdates          subsystems.DataSourceUpdateSink
	client                     *http.Client
	headers                    http.Header
	diagnosticsManager         *ldevents.DiagnosticsManager
	loggers                    ldlog.Loggers
	isInitialized              internal.AtomicBoolean
	streamReqCtx               gocontext.Context
	streamReqCancel            gocontext.CancelFunc
	storeStatusCh              <-chan interfaces.DataStoreStatus
	connectionAttemptStartTime ldtime.UnixMillisecondTime
	connectionAttemptLock      sync.Mutex
	readyOnce                  sync.Once
	closeOnce                  sync.Once
}

// NewStreamProcessor creates the internal implementation of the streaming data source.
func NewStreamProcessor(
	context subsystems.ClientContext,
	dataSourceUpdates subsystems.DataSourceUpdateSink,
	cfg StreamConfig,
) *StreamProcessor {
	// streamReqCancel is stored on sp and invoked from Close(); gosec's
	// G118 heuristic doesn't see the cross-scope call.
	//nolint:gosec // G118: cancel invoked from Close()
	streamReqCtx, streamReqCancel := gocontext.WithCancel(gocontext.Background())
	sp := &StreamProcessor{
		dataSourceUpdates: dataSourceUpdates,
		headers:           context.GetHTTP().DefaultHeaders,
		loggers:           context.GetLogging().Loggers,
		streamReqCtx:      streamReqCtx,
		streamReqCancel:   streamReqCancel,
		cfg:               cfg,
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
	if sp.dataSourceUpdates.GetDataStoreStatusProvider().IsStatusMonitoringEnabled() {
		sp.storeStatusCh = sp.dataSourceUpdates.GetDataStoreStatusProvider().AddStatusListener()
	}
	go sp.subscribe(closeWhenReady)
}

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

	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				// COVERAGE: stream.Events is only closed if the EventSource has been closed. That
				// happens after sp.streamReqCtx is cancelled (via the AfterFunc bridge in eventsource),
				// but our own sp.streamReqCtx.Done() arm below returns first-- so we should not
				// actually reach this point. Still, in case the channel is somehow closed unexpectedly,
				// we do want to terminate the loop.
				return
			}
			sp.logConnectionResult(true)

			processedEvent := true
			shouldRestart := false

			gotMalformedEvent := func(event es.Event, err error) {
				sp.loggers.Errorf(
					"Received streaming \"%s\" event with malformed JSON data (%s); will restart stream",
					event.Event(),
					err,
				)

				errorInfo := interfaces.DataSourceErrorInfo{
					Kind:    interfaces.DataSourceErrorKindInvalidData,
					Message: err.Error(),
					Time:    time.Now(),
				}
				sp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)

				shouldRestart = true // scenario 1 in error handling comments at top of file
				processedEvent = false
			}

			storeUpdateFailed := func(updateDesc string) {
				if sp.storeStatusCh != nil {
					sp.loggers.Errorf("Failed to store %s in data store; will try again once data store is working", updateDesc)
					// scenario 2a in error handling comments at top of file
				} else {
					sp.loggers.Errorf("Failed to store %s in data store; will restart stream until successful", updateDesc)
					shouldRestart = true // scenario 2b
					processedEvent = false
				}
			}

			switch event.Event() {
			case putEvent:
				put, err := parsePutData([]byte(event.Data()))
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}
				if sp.dataSourceUpdates.Init(put.Data) {
					if eventWithHeaders, ok := event.(es.EventWithHeaders); ok {
						if dataSourceUpdatesWithInitMetadata, ok :=
							sp.dataSourceUpdates.(subsystems.DataSourceUpdateSinkWithEnvironmentID); ok {
							initMetadata := internal.NewInitMetadataFromHeaders(eventWithHeaders.Headers())
							dataSourceUpdatesWithInitMetadata.SetEnvironmentID(initMetadata.GetEnvironmentID())
						}
					}
					sp.setInitializedAndNotifyClient(true, closeWhenReady)
				} else {
					storeUpdateFailed("initial streaming data")
				}

			case patchEvent:
				patch, err := parsePatchData([]byte(event.Data()))
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}
				if patch.Kind == nil {
					break // ignore unrecognized item type
				}
				if !sp.dataSourceUpdates.Upsert(patch.Kind, patch.Key, patch.Data) {
					storeUpdateFailed("streaming update of " + patch.Key)
				}

			case deleteEvent:
				del, err := parseDeleteData([]byte(event.Data()))
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}
				if del.Kind == nil {
					break // ignore unrecognized item type
				}
				deletedItem := ldstoretypes.ItemDescriptor{Version: del.Version, Item: nil}
				if !sp.dataSourceUpdates.Upsert(del.Kind, del.Key, deletedItem) {
					storeUpdateFailed("streaming deletion of " + del.Key)
				}

			default:
				sp.loggers.Infof("Unexpected event found in stream: %s", event.Event())
			}

			if processedEvent {
				sp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
			}
			if shouldRestart {
				stream.Restart()
			}

		case newStoreStatus := <-sp.storeStatusCh:
			if sp.loggers.IsDebugEnabled() {
				sp.loggers.Debugf("StreamProcessor received store status update: %+v", newStoreStatus)
			}
			if newStoreStatus.Available {
				// The store has just transitioned from unavailable to available (scenario 2a above)
				if newStoreStatus.NeedsRefresh {
					// The store is telling us that it can't guarantee that all of the latest data was cached.
					// So we'll restart the stream to ensure a full refresh.
					sp.loggers.Warn("Restarting stream to refresh data after data store outage")
					stream.Restart()
				}
				// All of the updates were cached and have been written to the store, so we don't need to
				// restart the stream. We just need to make sure the client knows we're initialized now
				// (in case the initial "put" was not stored).
				sp.setInitializedAndNotifyClient(true, closeWhenReady)
			}

		case <-sp.streamReqCtx.Done():
			return
		}
	}
}

func (sp *StreamProcessor) subscribe(closeWhenReady chan<- struct{}) {
	req, reqErr := http.NewRequestWithContext(
		sp.streamReqCtx, "GET",
		endpoints.AddPath(sp.cfg.URI, endpoints.StreamingRequestPath), nil,
	)
	if reqErr != nil {
		sp.loggers.Errorf(
			"Unable to create a stream request; this is not a network problem, most likely a bad base URI: %s",
			reqErr,
		)
		sp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateOff, interfaces.DataSourceErrorInfo{
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
	extendedInitialDelay := sp.cfg.ExtendedInitialReconnectDelay
	if extendedInitialDelay <= 0 {
		extendedInitialDelay = defaultStreamExtendedRetryDelay
	}
	retryResetInterval := sp.cfg.RetryResetInterval
	if retryResetInterval <= 0 {
		retryResetInterval = streamRetryResetInterval
	}

	defaultProfile := es.NewRetryProfile(
		es.RetryProfileBaseDelay(initialRetryDelay),
		es.RetryProfileMaxDelay(streamMaxRetryDelay),
		es.RetryProfileJitter(streamJitterRatio),
	)

	extendedProfile := es.NewRetryProfile(
		es.RetryProfileBaseDelay(extendedInitialDelay),
		es.RetryProfileMaxDelay(streamExtendedMaxRetryDelay),
		es.RetryProfileJitter(streamJitterRatio),
	)

	// loggedActivatedExtended gates the "engaging extended backoff" info log so
	// it fires at most once per Subscribe cycle. The library handles the
	// healthy-op reset back to normal internally (via
	// StreamOptionRetryResetInterval); we don't observe it, so we don't
	// re-log if the SDK re-transitions during a long-lived stream.
	loggedActivatedExtended := false

	errorHandler := func(err error) es.StreamErrorHandlerResult {
		sp.logConnectionResult(false)

		var class FailureClass
		var errorInfo interfaces.DataSourceErrorInfo

		if se, ok := err.(es.SubscriptionError); ok {
			errorInfo = interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindErrorResponse,
				StatusCode: se.Code,
				Time:       time.Now(),
			}
			class = classifyAndLogHTTPFailure(
				sp.loggers,
				httpErrorDescription(se.Code),
				streamingErrorContext,
				se.Code,
				streamingWillRetryMessage,
			)
		} else {
			errorInfo = interfaces.DataSourceErrorInfo{
				Kind:    interfaces.DataSourceErrorKindNetworkError,
				Message: err.Error(),
				Time:    time.Now(),
			}
			class = classifyAndLogTransportFailure(
				sp.loggers,
				err,
				streamingErrorContext,
				streamingWillRetryMessage,
			)
		}

		sp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)
		sp.logConnectionStarted()

		// No failure is permanently terminal. Unexpected failures engage the
		// extended-regime profile; the library keeps retrying at extended cadence
		// until a healthy-op reset (retryResetInterval of continuous connection)
		// reverts.
		result := es.StreamErrorHandlerResult{CloseNow: false}
		if class == FailureClassUnexpected {
			if !loggedActivatedExtended {
				sp.loggers.Info("Classified failure as UNEXPECTED; engaging extended backoff.")
				loggedActivatedExtended = true
			}
			result.ActivateProfile = extendedProfile
		}
		return result
	}

	stream, err := es.SubscribeWithRequestAndOptions(req,
		es.StreamOptionHTTPClient(sp.client),
		es.StreamOptionReadTimeout(streamReadTimeout),
		es.StreamOptionDefaultRetryProfile(defaultProfile),
		es.StreamOptionRegisterRetryProfile(extendedProfile),
		es.StreamOptionRetryResetInterval(retryResetInterval),
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
		sp.streamReqCancel()
		if sp.storeStatusCh != nil {
			sp.dataSourceUpdates.GetDataStoreStatusProvider().RemoveStatusListener(sp.storeStatusCh)
		}
		sp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateOff, interfaces.DataSourceErrorInfo{})
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

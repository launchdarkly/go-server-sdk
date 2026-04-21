package datasourcev2

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	es "github.com/launchdarkly/eventsource"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

const (
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
	client                     *http.Client
	headers                    http.Header
	diagnosticsManager         *ldevents.DiagnosticsManager
	loggers                    ldlog.Loggers
	connectionAttemptStartTime ldtime.UnixMillisecondTime
	connectionAttemptLock      sync.Mutex
	isClosed                   atomic.Bool
	halt                       chan struct{}
}

// NewStreamProcessor creates the internal implementation of the streaming data source.
func NewStreamProcessor(
	context subsystems.ClientContext,
	cfg datasource.StreamConfig,
) *StreamProcessor {
	sp := &StreamProcessor{
		headers: context.GetHTTP().DefaultHeaders,
		loggers: context.GetLogging().Loggers,
		halt:    make(chan struct{}),
		cfg:     cfg,
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

//nolint:revive // DataInitializer method.
func (sp *StreamProcessor) Name() string {
	return "StreamingDataSourceV2"
}

//nolint:revive // DataInitializer method.
func (sp *StreamProcessor) Fetch(ds subsystems.DataSelector, _ context.Context) (*subsystems.Basis, bool, error) {
	return nil, false, errors.New("StreamProcessor does not implement Fetch capability")
}

//nolint:revive // DataSynchronizer method.
func (sp *StreamProcessor) Sync(ds subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	resultChan := make(chan subsystems.DataSynchronizerResult, 100)

	if sp.isClosed.Load() {
		sp.loggers.Warnf("Streaming processor is already closed, not starting streaming")
		close(resultChan)
		return resultChan
	}

	sp.loggers.Info("Starting LaunchDarkly streaming connection")
	go sp.subscribe(ds, resultChan)

	return resultChan
}

// reportMalformedEvent logs a malformed-event error and pushes an Interrupted result on resultChan.
// Callers are responsible for resetting their own change-set builder and triggering a restart.
func (sp *StreamProcessor) reportMalformedEvent(
	event es.Event,
	err error,
	environmentID ldvalue.OptionalString,
	resultChan chan<- subsystems.DataSynchronizerResult,
) {
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

	resultChan <- subsystems.DataSynchronizerResult{
		State: interfaces.DataSourceStateInterrupted,
		Error: interfaces.DataSourceErrorInfo{
			Kind:    interfaces.DataSourceErrorKindInvalidData,
			Message: err.Error(),
			Time:    time.Now(),
		},
		EnvironmentID: environmentID,
	}
}

func (sp *StreamProcessor) consumeStream(stream *es.Stream, resultChan chan<- subsystems.DataSynchronizerResult) {
	// Consume remaining Events and Errors so we can garbage collect
	defer func() {
		for range stream.Events {
		} // COVERAGE: no way to cause this condition in unit tests
		if stream.Errors != nil {
			for range stream.Errors { // COVERAGE: no way to cause this condition in unit tests
			}
		}
	}()

	changeSetBuilder := subsystems.NewChangeSetBuilder()
	environmentID := ldvalue.OptionalString{}
	// fallbackRequested is set when the server's response headers carry x-ld-fd-fallback: true.
	// We finish applying the current payload before emitting the fallback signal, so evaluations
	// can serve the server-provided data while FDv1 takes over.
	fallbackRequested := false

	for {
		select {
		case event, ok := <-stream.Events:
			if !ok {
				close(resultChan)
				// COVERAGE: stream.Events is only closed if the EventSource has been closed. However, that
				// only happens when we have received from sp.halt, in which case we return immediately
				// after calling stream.Close(), terminating the for loop-- so we should not actually reach
				// this point. Still, in case the channel is somehow closed unexpectedly, we do want to
				// terminate the loop.
				return
			}

			sp.logConnectionResult(true)

			shouldRestart := false
			payloadApplied := false

			if eventWithHeaders, ok := event.(es.EventWithHeaders); ok {
				headers := eventWithHeaders.Headers()
				environmentID = internal.NewInitMetadataFromHeaders(headers).GetEnvironmentID()
				if isFDv1FallbackRequested(headers) {
					fallbackRequested = true
				}
			}

			gotMalformedEvent := func(event es.Event, err error) {
				// The protocol should "forget" anything that happens upon receiving an error.
				changeSetBuilder = subsystems.NewChangeSetBuilder()
				sp.reportMalformedEvent(event, err, environmentID, resultChan)
				shouldRestart = true // scenario 1 in error handling comments at top of file
			}

			switch subsystems.EventName(event.Event()) {
			case subsystems.EventHeartbeat:
				// Swallow the event and move on.
			case subsystems.EventServerIntent:

				var serverIntent subsystems.ServerIntent
				err := json.Unmarshal([]byte(event.Data()), &serverIntent)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}

				changeSetBuilder.Start(serverIntent)

				// IntentNone is a special case where we won't receive a payload-transferred event, so we will need
				// to instead immediately notify the client that we are initialized.
				if serverIntent.Payload.Code == subsystems.IntentNone {
					if err := changeSetBuilder.ExpectChanges(); err != nil {
						gotMalformedEvent(nil, err)
						break
					}

					resultChan <- subsystems.DataSynchronizerResult{
						State:          interfaces.DataSourceStateValid,
						EnvironmentID:  environmentID,
						FallbackToFDv1: fallbackRequested,
					}
					payloadApplied = true
					break
				}

			case subsystems.EventPutObject:
				var p subsystems.PutObject
				err := json.Unmarshal([]byte(event.Data()), &p)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}
				changeSetBuilder.AddPut(p.Kind, p.Key, p.Version, p.Object)
			case subsystems.EventDeleteObject:
				var d subsystems.DeleteObject
				err := json.Unmarshal([]byte(event.Data()), &d)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}
				changeSetBuilder.AddDelete(d.Kind, d.Key, d.Version)
			case subsystems.EventGoodbye:
				var goodbye subsystems.Goodbye
				err := json.Unmarshal([]byte(event.Data()), &goodbye)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}

				if !goodbye.Silent {
					sp.loggers.Errorf("SSE server received error: %s (%v)", goodbye.Reason, goodbye.Catastrophe)
				}
			case subsystems.EventError:
				var errorData subsystems.Error
				err := json.Unmarshal([]byte(event.Data()), &errorData)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}

				sp.loggers.Errorf("Error on %s: %s", errorData.PayloadID, errorData.Reason)

				// The protocol should "reset" any previous change events it
				// has received, but should continue to operate under the
				// assumption the last server intent was in effect.
				//
				// The server may choose to send a new server-intent, at which
				// point we will set that as well.
				changeSetBuilder.Reset()

			case subsystems.EventPayloadTransferred:
				var selector subsystems.Selector
				err := json.Unmarshal([]byte(event.Data()), &selector)
				if err != nil {
					gotMalformedEvent(event, err)
					break
				}

				// After calling Finish, the builder is ready to receive a new changeset.
				changeSet, err := changeSetBuilder.Finish(selector)
				if err != nil {
					gotMalformedEvent(nil, err)
					break
				}

				resultChan <- subsystems.DataSynchronizerResult{
					ChangeSet:      changeSet,
					State:          interfaces.DataSourceStateValid,
					EnvironmentID:  environmentID,
					FallbackToFDv1: fallbackRequested,
				}
				payloadApplied = true

			default:
				sp.loggers.Infof("Unexpected event found in stream: %s", event.Event())
			}

			if shouldRestart {
				stream.Restart()
			}

			// Once a payload has been applied with a pending FDv1 fallback signal, the Valid
			// result emitted above carries FallbackToFDv1=true; close the stream so we stop
			// consuming. Events that don't complete a payload leave payloadApplied false so we
			// keep consuming (fallbackRequested persists across iterations).
			if fallbackRequested && payloadApplied {
				stream.Close()
				return
			}

		case <-sp.halt:
			stream.Close()
			return
		}
	}
}

func (sp *StreamProcessor) subscribe(ds subsystems.DataSelector, resultChan chan<- subsystems.DataSynchronizerResult) {
	path := endpoints.AddPath(sp.cfg.URI, endpoints.StreamingRequestV2Path)
	req, reqErr := http.NewRequest("GET", path, nil)
	if reqErr != nil {
		sp.loggers.Errorf(
			"Unable to create a stream request; this is not a network problem, most likely a bad base URI: %s",
			reqErr,
		)
		resultChan <- subsystems.DataSynchronizerResult{
			State: interfaces.DataSourceStateOff,
			Error: interfaces.DataSourceErrorInfo{
				Kind:    interfaces.DataSourceErrorKindUnknown,
				Message: reqErr.Error(),
				Time:    time.Now(),
			},
		}
		close(resultChan)
		sp.logConnectionResult(false)
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
			environmentID := internal.NewInitMetadataFromHeaders(se.Header).GetEnvironmentID()

			errorInfo := interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindErrorResponse,
				StatusCode: se.Code,
				Time:       time.Now(),
			}

			if isFDv1FallbackRequested(se.Header) {
				resultChan <- subsystems.DataSynchronizerResult{
					State:          interfaces.DataSourceStateOff,
					Error:          errorInfo,
					FallbackToFDv1: true,
					EnvironmentID:  environmentID,
				}
				return es.StreamErrorHandlerResult{CloseNow: true}
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
				resultChan <- subsystems.DataSynchronizerResult{
					State:         interfaces.DataSourceStateInterrupted,
					Error:         errorInfo,
					EnvironmentID: environmentID,
				}
				return es.StreamErrorHandlerResult{CloseNow: false}
			}
			resultChan <- subsystems.DataSynchronizerResult{
				State:         interfaces.DataSourceStateOff,
				Error:         errorInfo,
				EnvironmentID: environmentID,
			}
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
		resultChan <- subsystems.DataSynchronizerResult{
			State: interfaces.DataSourceStateInterrupted,
			Error: errorInfo,
		}
		sp.logConnectionStarted()
		return es.StreamErrorHandlerResult{CloseNow: false}
	}

	stream, err := es.SubscribeWithRequestAndOptions(req,
		es.StreamOptionDynamicQueryParams(func(existing url.Values) url.Values {
			if selector := ds.Selector(); selector.IsDefined() {
				existing.Set("basis", selector.State())
			}

			return existing
		}),
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

		resultChan <- subsystems.DataSynchronizerResult{
			State: interfaces.DataSourceStateOff,
			Error: interfaces.DataSourceErrorInfo{
				Kind:    interfaces.DataSourceErrorKindUnknown,
				Message: err.Error(),
				Time:    time.Now(),
			},
		}
		close(resultChan)

		return
	}

	sp.consumeStream(stream, resultChan)
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
	if swapped := sp.isClosed.CompareAndSwap(false, true); swapped {
		close(sp.halt)
	}
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

// vim: foldmethod=marker foldlevel=0

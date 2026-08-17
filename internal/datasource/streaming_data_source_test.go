package datasource

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	"github.com/launchdarkly/eventsource"
	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	briefDelay                     = time.Millisecond * 50
	streamProcessorTestHeaderName  = "my-header"
	streamProcessorTestHeaderValue = "my-value"
)

type streamingTestParams struct {
	events   chan<- eventsource.Event
	updates  *mocks.MockDataSourceUpdates
	stream   httphelpers.SSEStreamControl
	requests <-chan httphelpers.HTTPRequestInfo
	mockLog  *ldlogtest.MockLog
}

func runStreamingTest(
	t *testing.T,
	initialData *ldservices.ServerSDKData,
	test func(streamingTestParams),
) {
	runStreamingTestWithConfiguration(t, initialData, nil, test)
}

func runStreamingTestWithConfiguration(
	t *testing.T,
	initialData *ldservices.ServerSDKData,
	configureUpdates func(*mocks.MockDataSourceUpdates),
	test func(streamingTestParams),
) {
	events := make(chan eventsource.Event, 1000)
	streamHandler, stream := ldservices.ServerSideStreamingServiceHandler(initialData.ToPutEvent())

	// We provide a second stream handler so that if the first stream gets explicitly closed by a test,
	// we'll be able to able to reconnect (a closed stream handler can't be reused)
	extraStreamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData.ToPutEvent())

	handler, requestsCh := httphelpers.RecordingHandler(
		httphelpers.SequentialHandler(streamHandler, extraStreamHandler),
	)

	headers := make(http.Header)
	headers.Set(streamProcessorTestHeaderName, streamProcessorTestHeaderValue)
	mockLog := ldlogtest.NewMockLog()
	mockLog.Loggers.SetMinLevel(ldlog.Debug)
	defer mockLog.DumpIfTestFailed(t)
	context := sharedtest.NewTestContext("", &subsystems.HTTPConfiguration{DefaultHeaders: headers},
		&subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})

	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
			if configureUpdates != nil {
				configureUpdates(dataSourceUpdates)
			}

			sp := NewStreamProcessor(
				context,
				dataSourceUpdates,
				StreamConfig{
					URI:                   streamServer.URL,
					InitialReconnectDelay: briefDelay,
				},
			)
			defer sp.Close()

			closeWhenReady := make(chan struct{})

			sp.Start(closeWhenReady)

			if !th.AssertChannelClosed(t, closeWhenReady, time.Second, "timed out waiting for data source to start") {
				return
			}

			params := streamingTestParams{events, dataSourceUpdates, stream, requestsCh, mockLog}
			test(params)
		})
	})
}

func TestStreamProcessor(t *testing.T) {
	t.Parallel()
	initialData := ldservices.NewServerSDKData().
		Flags(ldservices.KeyAndVersionItem("my-flag", 2)).
		Segments(ldservices.KeyAndVersionItem("my-segment", 2))
	timeout := 3 * time.Second

	t.Run("configured headers are passed in request", func(t *testing.T) {
		runStreamingTest(t, initialData, func(p streamingTestParams) {
			r := <-p.requests
			assert.Equal(t, streamProcessorTestHeaderValue, r.Request.Header.Get(streamProcessorTestHeaderName))
		})
	})

	t.Run("initial put", func(t *testing.T) {
		runStreamingTest(t, initialData, func(p streamingTestParams) {
			p.updates.DataStore.WaitForInit(t, initialData, timeout)
		})
	})

	t.Run("patch flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: patchEvent,
				Data: `{"path": "/flags/my-flag", "data": {"key": "my-flag", "version": 3}}`})

			p.updates.DataStore.WaitForUpsert(t, datakinds.Features, "my-flag", 3, timeout)
		})
	})

	t.Run("delete flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: deleteEvent,
				Data: `{"path": "/flags/my-flag", "version": 4}`})

			p.updates.DataStore.WaitForDelete(t, datakinds.Features, "my-flag", 4, timeout)
		})
	})

	t.Run("patch segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: patchEvent,
				Data: `{"path": "/segments/my-segment", "data": {"key": "my-segment", "version": 7}}`})

			p.updates.DataStore.WaitForUpsert(t, datakinds.Segments, "my-segment", 7, timeout)
		})
	})

	t.Run("delete segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: deleteEvent,
				Data: `{"path": "/segments/my-segment", "version": 8}`})

			p.updates.DataStore.WaitForDelete(t, datakinds.Segments, "my-segment", 8, timeout)
		})
	})
}

func TestStreamProcessorRecoverableErrorsCauseStreamRestart(t *testing.T) {
	t.Parallel()

	expectRestart := func(t *testing.T, p streamingTestParams) {
		<-p.requests // ignore initial HTTP request
		th.RequireValue(t, p.requests, time.Millisecond*300, "expected stream restart, did not see one")
		p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid)       // the initial connection
		p.updates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted) // the error
		p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid)       // the restarted connection
	}

	for _, status := range []int{400, 500} {
		t.Run(fmt.Sprintf("HTTP status %d", status), func(t *testing.T) {
			testStreamProcessorRecoverableHTTPError(t, status)
		})
	}

	t.Run("dropped connection", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.EndAll()
			<-time.After(300 * time.Millisecond)
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Warn, ".*Error in stream connection")
		})
	})

	t.Run("put with malformed JSON", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: putEvent, Data: `{"path": "/", "data": }"`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})

	t.Run("put with well-formed JSON but malformed data model item", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: putEvent,
				Data: `{"path": "/", "data": {"flags": {"flagkey": {"key": [], "version": true}}, "segments": {}}}`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})

	t.Run("patch with omitted path", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: patchEvent,
				Data: `{"data": {"key": "flagkey"}}`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*a required property \"path\" was missing.*will restart")
		})
	})

	t.Run("patch with malformed JSON", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: patchEvent, Data: `{"path":"/flags/flagkey"`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})

	t.Run("patch with well-formed JSON but malformed data model item", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: patchEvent,
				Data: `{"path":"/flags/flagkey", "data": {"key": [], "version": true}}`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})

	t.Run("delete with omitted path", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: deleteEvent, Data: `{"version": 8}`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*a required property \"path\" was missing.*will restart")
		})
	})

	t.Run("patch with malformed JSON", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: deleteEvent, Data: `{"path":"/flags/flagkey"`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})
}

// Under the RETRY spec (SDK-2775), 401 / 403 / other 4xx are no longer terminal --
// they engage an extended-regime backoff but keep retrying indefinitely. The SDK
// transitions to Interrupted (not Off) and does not close the initialization
// channel. This test replaces the pre-RETRY TestStreamProcessorUnrecoverableErrors
// CauseStreamShutdown, which asserted the old (permanent-stop) behavior.
func TestStreamProcessorUnexpectedErrorsEngageExtendedRegimeAndKeepRetrying(t *testing.T) {
	for _, status := range []int{401, 403, 404} {
		t.Run(fmt.Sprintf("HTTP status %d", status), func(t *testing.T) {
			testStreamProcessorUnexpectedHTTPError(t, status)
		})
	}
}

func TestStreamProcessorUnrecognizedDataIsIgnored(t *testing.T) {
	t.Parallel()

	expectNoRestart := func(t *testing.T, p streamingTestParams) {
		<-p.requests // ignore initial HTTP request

		th.AssertNoMoreValues(t, p.requests, time.Millisecond*100, "stream restarted unexpectedly")

		assert.Len(t, p.mockLog.GetOutput(ldlog.Error), 0)

		p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid) // the initial connection
		th.AssertNoMoreValues(t, p.updates.Statuses, time.Millisecond*100, "unexpected data source status change")
	}

	t.Run("patch with unrecognized path", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: patchEvent,
				Data: `{"path": "/wrong", "data": {"key": "flagkey"}}`})
			expectNoRestart(t, p)
		})
	})

	t.Run("delete with unrecognized path", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: deleteEvent,
				Data: `{"path": "/wrong", "version": 8}`})
			expectNoRestart(t, p)
		})
	})

	t.Run("unknown message type", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: "weird-event", Data: `x`})
			expectNoRestart(t, p)
		})
	})
}

func TestStreamProcessorStoreUpdateFailureWithStatusTracking(t *testing.T) {
	// Normally, a data store can only fail if it is a persistent store that uses the standard
	// PersistentDataStore framework, in which case store status tracking is available and the
	// stream will only restart after a store failure if the store tells it to.

	fakeError := errors.New("sorry")

	expectStoreFailureAndRecovery := func(t *testing.T, p streamingTestParams) {
		<-p.requests // ignore initial HTTP request

		th.AssertNoMoreValues(t, p.requests, time.Millisecond*100, "stream restarted unexpectedly")

		p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid) // the initial connection
		p.mockLog.AssertMessageMatch(t, true, ldlog.Error,
			".*Failed to store.*will try again once data store is working")

		p.updates.DataStore.SetFakeError(nil)
		p.updates.UpdateStoreStatus(interfaces.DataStoreStatus{Available: true, NeedsRefresh: true})

		th.RequireValue(t, p.requests, time.Millisecond*300, "expected stream restart, did not see one")

		p.mockLog.AssertMessageMatch(t, true, ldlog.Warn, "Restarting stream.*after data store outage")
	}

	t.Run("Init fails on put", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.updates.DataStore.SetFakeError(fakeError)

			p.stream.Send(ldservices.NewServerSDKData().ToPutEvent())

			expectStoreFailureAndRecovery(t, p)
		})
	})

	t.Run("Upsert fails on patch", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.updates.DataStore.SetFakeError(fakeError)

			p.stream.Send(httphelpers.SSEEvent{Event: patchEvent,
				Data: `{"path": "/flags/my-flag", "data": {"key": "my-flag", "version": 3}}`})

			expectStoreFailureAndRecovery(t, p)
		})
	})

	t.Run("Upsert fails on delete", func(t *testing.T) {
		runStreamingTest(t, ldservices.NewServerSDKData(), func(p streamingTestParams) {
			p.updates.DataStore.SetFakeError(fakeError)

			p.stream.Send(httphelpers.SSEEvent{Event: deleteEvent,
				Data: `{"path": "/flags/my-flag", "version": 4}`})

			expectStoreFailureAndRecovery(t, p)
		})
	})
}

func TestStreamProcessorStoreUpdateFailureWithoutStatusTracking(t *testing.T) {
	// In the unusual case where a store update fails but the store does not support status tracking
	// (like if it's some custom implementation), the store should restart immediately after the failure.
	// We're only testing this case with a single kind of event because it doesn't really matter which
	// kind of operation failed in this case.

	fakeError := errors.New("sorry")

	initialData := ldservices.NewServerSDKData()
	noStatusMonitoring := func(u *mocks.MockDataSourceUpdates) {
		u.DataStore.SetStatusMonitoringEnabled(false)
	}

	runStreamingTestWithConfiguration(t, initialData, noStatusMonitoring, func(p streamingTestParams) {
		<-p.requests // ignore initial HTTP request

		p.updates.DataStore.SetFakeError(fakeError)

		p.stream.Send(initialData.ToPutEvent())

		th.RequireValue(t, p.requests, time.Millisecond*300, "expected stream restart, did not see one")

		p.mockLog.AssertMessageMatch(t, true, ldlog.Error, "Failed to store.*will restart stream")
	})

}

func testStreamProcessorUnexpectedHTTPError(t *testing.T, statusCode int) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	// Record the request stream so we can verify the SDK keeps retrying after the failure.
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(statusCode))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
			id := ldevents.NewDiagnosticID(sharedtest.TestSDKKey)
			diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
			context := &internal.ClientContextImpl{
				BasicClientContext: subsystems.BasicClientContext{
					SDKKey:  sharedtest.TestSDKKey,
					Logging: subsystems.LoggingConfiguration{Loggers: mockLog.Loggers},
				},
				DiagnosticsManager: diagnosticsManager,
			}

			// Short retry delays so we can observe at least two attempts within the
			// assertion window. The extended-regime profile activates immediately on the
			// first failure (per the RETRY spec -- no grace period for initial-connection
			// unexpected classifications), so we need to shorten ExtendedInitialReconnectDelay
			// too, not just the normal InitialReconnectDelay.
			sp := NewStreamProcessor(context, dataSourceUpdates, StreamConfig{
				URI:                           ts.URL,
				InitialReconnectDelay:         10 * time.Millisecond,
				ExtendedInitialReconnectDelay: 20 * time.Millisecond,
			})
			defer sp.Close()

			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			// Initialization must not complete: no permanent stop, no successful put.
			// The client's Start() would time out via StartWaitTimeMS in production; here
			// we just assert the channel stays open.
			select {
			case <-closeWhenReady:
				t.Fatal("closeWhenReady should not be closed -- RETRY §1.2.1 forbids permanent stops on 4xx")
			case <-time.After(time.Second):
			}

			// The mock data source updates records the raw UpdateStatus calls made
			// by the processor -- so we see the Interrupted call the processor tried
			// to make. (The real DataSourceUpdateSinkImpl would clamp it to
			// Initializing since we never reached Valid; that's tested at the
			// LDClient-level in the RETRY end-to-end tests. Here we just verify the
			// processor emitted the correct call.)
			status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
			assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, status.LastError.Kind)
			assert.Equal(t, statusCode, status.LastError.StatusCode)

			// Confirm the SDK actually retried (at least a second request landed at the mock server).
			// The initial-regime retry delay is 10ms above, so 500ms is plenty of budget.
			require.Eventually(t, func() bool { return len(requestsCh) >= 2 },
				500*time.Millisecond, 10*time.Millisecond,
				"expected the SDK to retry after the unexpected-classified error")
		})
	})
}

func testStreamProcessorRecoverableHTTPError(t *testing.T, statusCode int) {
	initialData := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData.ToPutEvent())
	sequentialHandler := httphelpers.SequentialHandler(
		httphelpers.HandlerWithStatus(statusCode), // fails the first time
		streamHandler, // then gets a valid stream
	)
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	httphelpers.WithServer(sequentialHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
			id := ldevents.NewDiagnosticID(sharedtest.TestSDKKey)
			diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
			context := &internal.ClientContextImpl{
				BasicClientContext: subsystems.BasicClientContext{
					SDKKey:  sharedtest.TestSDKKey,
					Logging: subsystems.LoggingConfiguration{Loggers: mockLog.Loggers},
				},
				DiagnosticsManager: diagnosticsManager,
			}

			sp := NewStreamProcessor(context, dataSourceUpdates, StreamConfig{URI: ts.URL, InitialReconnectDelay: briefDelay})
			defer sp.Close()

			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			th.AssertChannelClosed(t, closeWhenReady, time.Second*3, "Should have successfully retried before now")

			event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
			assert.Equal(t, 2, event.GetByKey("streamInits").Count())
			assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))
			assert.Equal(t, ldvalue.Bool(false), event.GetByKey("streamInits").GetByIndex(1).GetByKey("failed"))

			// should have gotten two status updates: first for the error, then the success - note that we're checking
			// here for Interrupted because that's how the StreamProcessor reports the error, even though in the public
			// API it would show up as Initializing because it was still initializing
			status1 := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
			assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, status1.LastError.Kind)
			assert.Equal(t, statusCode, status1.LastError.StatusCode)
			_ = dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateValid)
		})
	})
}

func TestStreamProcessorUsesHTTPClientFactory(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
			context := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

			sp := NewStreamProcessor(context, dataSourceUpdates, StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: briefDelay,
			})

			defer sp.Close()
			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			r := <-requestsCh

			assert.Equal(t, "/all/transformed", r.Request.URL.Path)
		})
	})
}

func TestStreamProcessorDoesNotUseConfiguredTimeoutAsReadTimeout(t *testing.T) {
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(ldservices.NewServerSDKData().ToPutEvent())
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
			httpClientFactory := func() *http.Client {
				c := *http.DefaultClient
				c.Timeout = 200 * time.Millisecond
				return &c
			}
			httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
			context := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

			sp := NewStreamProcessor(context, dataSourceUpdates, StreamConfig{URI: ts.URL, InitialReconnectDelay: briefDelay})
			defer sp.Close()
			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			<-time.After(500 * time.Millisecond)
			assert.Equal(t, 1, len(requestsCh))
		})
	})
}

func TestStreamProcessorRestartsStreamIfStoreNeedsRefresh(t *testing.T) {
	initialData := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 1))
	updatedData := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	streamHandler1, _ := ldservices.ServerSideStreamingServiceHandler(initialData.ToPutEvent())
	streamHandler2, _ := ldservices.ServerSideStreamingServiceHandler(updatedData.ToPutEvent())
	streamHandler := httphelpers.SequentialHandler(streamHandler1, streamHandler2)

	httphelpers.WithServer(streamHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(updates *mocks.MockDataSourceUpdates) {
			sp := NewStreamProcessor(sharedtest.BasicClientContext(), updates, StreamConfig{URI: ts.URL, InitialReconnectDelay: briefDelay})
			defer sp.Close()

			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			// Wait until the stream has received data and put it in the store
			updates.DataStore.WaitForInit(t, initialData, 3*time.Second)

			// Make the data store simulate an outage and recovery with NeedsRefresh: true
			updates.UpdateStoreStatus(interfaces.DataStoreStatus{Available: false})
			updates.UpdateStoreStatus(interfaces.DataStoreStatus{Available: true, NeedsRefresh: true})

			// When the stream restarts, it'll call Init with the updated data from streamHandler1
			updates.DataStore.WaitForInit(t, updatedData, 3*time.Second)
		})
	})
}

func TestMalformedStreamBaseURI(t *testing.T) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	clientContext := &internal.ClientContextImpl{
		BasicClientContext: subsystems.BasicClientContext{
			SDKKey:  sharedtest.TestSDKKey,
			Logging: subsystems.LoggingConfiguration{Loggers: mockLog.Loggers},
		},
	}
	withMockDataSourceUpdates(func(updates *mocks.MockDataSourceUpdates) {
		sp := NewStreamProcessor(clientContext, updates, StreamConfig{
			URI:                   ":/",
			InitialReconnectDelay: briefDelay,
		})
		defer sp.Close()

		closeWhenReady := make(chan struct{})
		sp.Start(closeWhenReady)

		status := updates.RequireStatusOf(t, interfaces.DataSourceStateOff)
		assert.Equal(t, interfaces.DataSourceErrorKindUnknown, status.LastError.Kind)
		<-closeWhenReady

		mockLog.AssertMessageMatch(t, true, ldlog.Error, "Unable to create a stream request")
	})
}

func TestStreamProcessorAppendsFilterParameter(t *testing.T) {
	testWithFilters(t, func(t *testing.T, filter filterTest) {
		handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data

		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {

				sp := NewStreamProcessor(sharedtest.BasicClientContext(), dataSourceUpdates, StreamConfig{
					URI:                   ts.URL,
					InitialReconnectDelay: briefDelay,
					FilterKey:             filter.key,
				})

				defer sp.Close()
				closeWhenReady := make(chan struct{})
				sp.Start(closeWhenReady)

				r := <-requestsCh

				assert.Equal(t, filter.query, r.Request.URL.RawQuery)
			})
		})
	})
}

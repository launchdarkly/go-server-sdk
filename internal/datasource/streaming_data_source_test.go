package datasource

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/ldservices"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/eventsource"
)

const (
	briefDelay                     = time.Millisecond * 50
	streamProcessorTestHeaderName  = "my-header"
	streamProcessorTestHeaderValue = "my-value"
)

type streamingTestParams struct {
	events   chan<- eventsource.Event
	updates  *sharedtest.MockDataSourceUpdates
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
	configureUpdates func(*sharedtest.MockDataSourceUpdates),
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
	context := sharedtest.NewTestContext("", sharedtest.TestHTTPConfigWithHeaders(headers),
		sharedtest.TestLoggingConfigWithLoggers(mockLog.Loggers))

	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			if configureUpdates != nil {
				configureUpdates(dataSourceUpdates)
			}

			sp := NewStreamProcessor(
				context,
				dataSourceUpdates,
				streamServer.URL,
				briefDelay,
			)
			defer sp.Close()

			closeWhenReady := make(chan struct{})

			sp.Start(closeWhenReady)

			select {
			case <-closeWhenReady:
			case <-time.After(time.Second):
				assert.Fail(t, "start timeout")
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
		Flags(ldservices.FlagOrSegment("my-flag", 2)).
		Segments(ldservices.FlagOrSegment("my-segment", 2))
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
		select {
		case <-p.requests:
			break
		case <-time.After(time.Millisecond * 300):
			assert.Fail(t, "expected stream restart, did not see one")
			return
		}
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
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*missing item path.*will restart")
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
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*missing item path.*will restart")
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

func TestStreamProcessorUnrecoverableErrorsCauseStreamShutdown(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(fmt.Sprintf("HTTP status %d", status), func(t *testing.T) {
			testStreamProcessorUnrecoverableHTTPError(t, status)
		})
	}
}

func TestStreamProcessorUnrecognizedDataIsIgnored(t *testing.T) {
	t.Parallel()

	expectNoRestart := func(t *testing.T, p streamingTestParams) {
		<-p.requests // ignore initial HTTP request

		select {
		case <-p.requests:
			assert.Fail(t, "stream restarted unexpectedly")
		case <-time.After(time.Millisecond * 100):
		}

		assert.Len(t, p.mockLog.GetOutput(ldlog.Error), 0)

		p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid) // the initial connection
		select {
		case status := <-p.updates.Statuses:
			assert.Fail(t, "unexpected data source status change", "new status: %+v", status)
		case <-time.After(time.Millisecond * 100):
		}
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

		select {
		case <-p.requests:
			assert.Fail(t, "stream restarted unexpectedly")
		case <-time.After(time.Millisecond * 100):
		}

		p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid) // the initial connection
		p.mockLog.AssertMessageMatch(t, true, ldlog.Error,
			".*Failed to store.*will try again once data store is working")

		p.updates.DataStore.SetFakeError(nil)
		p.updates.UpdateStoreStatus(interfaces.DataStoreStatus{Available: true, NeedsRefresh: true})

		select {
		case <-p.requests:
			break
		case <-time.After(time.Millisecond * 300):
			assert.Fail(t, "expected stream restart, did not see one")
			return
		}

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
	noStatusMonitoring := func(u *sharedtest.MockDataSourceUpdates) {
		u.DataStore.SetStatusMonitoringEnabled(false)
	}

	runStreamingTestWithConfiguration(t, initialData, noStatusMonitoring, func(p streamingTestParams) {
		<-p.requests // ignore initial HTTP request

		p.updates.DataStore.SetFakeError(fakeError)

		p.stream.Send(initialData.ToPutEvent())

		select {
		case <-p.requests:
			break
		case <-time.After(time.Millisecond * 300):
			assert.Fail(t, "expected stream restart, did not see one")
			return
		}

		p.mockLog.AssertMessageMatch(t, true, ldlog.Error, "Failed to store.*will restart stream")
	})
}

func testStreamProcessorUnrecoverableHTTPError(t *testing.T, statusCode int) {
	httphelpers.WithServer(httphelpers.HandlerWithStatus(statusCode), func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			id := ldevents.NewDiagnosticID(testSDKKey)
			diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
			context := sharedtest.NewClientContextWithDiagnostics(testSDKKey, nil, nil, diagnosticsManager)

			sp := NewStreamProcessor(context, dataSourceUpdates, ts.URL, time.Second)
			defer sp.Close()

			closeWhenReady := make(chan struct{})

			sp.Start(closeWhenReady)

			select {
			case <-closeWhenReady:
				assert.False(t, sp.IsInitialized())
			case <-time.After(time.Second * 3):
				assert.Fail(t, "Initialization shouldn't block after this error")
			}

			event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
			assert.Equal(t, 1, event.GetByKey("streamInits").Count())
			assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))

			status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateOff)
			assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, status.LastError.Kind)
			assert.Equal(t, statusCode, status.LastError.StatusCode)
		})
	})
}

func testStreamProcessorRecoverableHTTPError(t *testing.T, statusCode int) {
	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData.ToPutEvent())
	sequentialHandler := httphelpers.SequentialHandler(
		httphelpers.HandlerWithStatus(statusCode), // fails the first time
		streamHandler, // then gets a valid stream
	)
	httphelpers.WithServer(sequentialHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			id := ldevents.NewDiagnosticID(testSDKKey)
			diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
			context := sharedtest.NewClientContextWithDiagnostics(testSDKKey, nil, nil, diagnosticsManager)

			sp := NewStreamProcessor(context, dataSourceUpdates, ts.URL, briefDelay)
			defer sp.Close()

			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			select {
			case <-closeWhenReady:
				assert.True(t, sp.IsInitialized())
			case <-time.After(time.Second * 3):
				assert.Fail(t, "Should have successfully retried before now")
			}

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
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			httpConfig := internal.HTTPConfigurationImpl{HTTPClientFactory: httpClientFactory}
			context := sharedtest.NewTestContext(testSDKKey, httpConfig, sharedtest.TestLoggingConfig())

			sp := NewStreamProcessor(context, dataSourceUpdates, ts.URL, briefDelay)
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
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			httpClientFactory := func() *http.Client {
				c := *http.DefaultClient
				c.Timeout = 200 * time.Millisecond
				return &c
			}
			httpConfig := internal.HTTPConfigurationImpl{HTTPClientFactory: httpClientFactory}
			context := sharedtest.NewTestContext(testSDKKey, httpConfig, sharedtest.TestLoggingConfig())

			sp := NewStreamProcessor(context, dataSourceUpdates, ts.URL, briefDelay)
			defer sp.Close()
			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			<-time.After(500 * time.Millisecond)
			assert.Equal(t, 1, len(requestsCh))
		})
	})
}

func TestStreamProcessorRestartsStreamIfStoreNeedsRefresh(t *testing.T) {
	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 1))
	updatedData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	streamHandler1, _ := ldservices.ServerSideStreamingServiceHandler(initialData.ToPutEvent())
	streamHandler2, _ := ldservices.ServerSideStreamingServiceHandler(updatedData.ToPutEvent())
	streamHandler := httphelpers.SequentialHandler(streamHandler1, streamHandler2)

	httphelpers.WithServer(streamHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(updates *sharedtest.MockDataSourceUpdates) {
			sp := NewStreamProcessor(basicClientContext(), updates, ts.URL, briefDelay)
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

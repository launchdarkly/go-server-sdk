package datasourcev2

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/eventsource"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	"github.com/stretchr/testify/assert"
)

const (
	putEvent          = "put"
	putObjectEvent    = "put-object"
	deleteObjectEvent = "delete-object"

	briefDelay                     = time.Millisecond * 50
	streamProcessorTestHeaderName  = "my-header"
	streamProcessorTestHeaderValue = "my-value"
)

type streamingTestParams struct {
	events         chan<- eventsource.Event
	statusReporter *mocks.MockStatusReporter
	stream         httphelpers.SSEStreamControl
	requests       <-chan httphelpers.HTTPRequestInfo
	mockLog        *ldlogtest.MockLog
}

func TestMalformedStreamBaseURI(t *testing.T) {
	dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	statusReporter := mocks.NewMockStatusReporter()

	sp := NewStreamProcessor(
		sharedtest.BasicClientContext(),
		dd,
		statusReporter,
		datasource.StreamConfig{
			URI:                   ":/",
			InitialReconnectDelay: time.Millisecond * 50,
			FilterKey:             "filter-value",
		},
	)

	defer sp.Close()
	closeWhenReady := make(chan struct{})
	sp.Sync(closeWhenReady)

	status := statusReporter.RequireStatusOf(t, interfaces.DataSourceStateOff)
	assert.Equal(t, interfaces.DataSourceErrorKindUnknown, status.LastError.Kind)
	<-closeWhenReady
}

func TestStreamingProcessorAppendsFilterParameter(t *testing.T) {
	dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	statusReporter := mocks.NewMockStatusReporter()

	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			dd,
			statusReporter,
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
				FilterKey:             "filter-value",
			},
		)

		defer sp.Close()
		closeWhenReady := make(chan struct{})
		sp.Sync(closeWhenReady)

		r := <-requestsCh

		assert.Equal(t, "filter-value", r.Request.URL.Query().Get("filter"))
	})
}

func TestStreamingProcessorDoesNotUseConfiguredTimeoutAsReadTimeout(t *testing.T) {
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(ldservicesv2.NewServerSDKData().ToPutObjects()).
		WithTransferred(1)
	streamHandler, stream := ldservices.ServerSideStreamingServiceHandler(protocol.Next())
	protocol.Enqueue(stream)

	dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	statusReporter := mocks.NewMockStatusReporter()

	handler, requestsCh := httphelpers.RecordingHandler(streamHandler) // we don't care about getting valid stream data
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		httpClientFactory := func() *http.Client {
			c := *http.DefaultClient
			c.Timeout = 200 * time.Millisecond
			return &c
		}
		httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
		context := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

		sp := NewStreamProcessor(
			context,
			dd,
			statusReporter,
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
				FilterKey:             "filter-value",
			},
		)

		defer sp.Close()
		closeWhenReady := make(chan struct{})
		sp.Sync(closeWhenReady)

		<-time.After(500 * time.Millisecond)
		assert.Equal(t, 1, len(requestsCh))
	})
}

func TestStreamProcessorRecoverableErrorsCauseStreamRestart(t *testing.T) {
	t.Parallel()

	expectRestart := func(t *testing.T, p streamingTestParams) {
		<-p.requests // ignore initial HTTP request
		th.RequireValue(t, p.requests, time.Millisecond*300, "expected stream restart, did not see one")
		p.statusReporter.RequireStatusOf(t, interfaces.DataSourceStateValid)       // the initial connection
		p.statusReporter.RequireStatusOf(t, interfaces.DataSourceStateInterrupted) // the error
		p.statusReporter.RequireStatusOf(t, interfaces.DataSourceStateValid)       // the restarted connection
	}

	for _, status := range []int{400, 500} {
		t.Run(fmt.Sprintf("HTTP status %d", status), func(t *testing.T) {
			testStreamProcessorRecoverableHTTPError(t, status)
		})
	}

	t.Run("dropped connection", func(t *testing.T) {
		runStreamingTest(t, ldservicesv2.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.EndAll()
			<-time.After(300 * time.Millisecond)
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Warn, ".*Error in stream connection")
		})
	})

	t.Run("unexpected event type", func(t *testing.T) {
		runStreamingTest(t, ldservicesv2.NewServerSDKData(), func(p streamingTestParams) {
			// Send an old v1 format event.
			p.stream.Send(httphelpers.SSEEvent{Event: putEvent, Data: `{"path": "/", "data": }"`})
			p.stream.EndAll()
			<-time.After(300 * time.Millisecond)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Info, "Unexpected event found in stream")
		})
	})

	t.Run("put-object with malformed JSON", func(t *testing.T) {
		runStreamingTest(t, ldservicesv2.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: putObjectEvent, Data: `{"data": }`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})

	t.Run("put-object with well-formed JSON but malformed data model item", func(t *testing.T) {
		runStreamingTest(t, ldservicesv2.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: putObjectEvent,
				Data: `{"version": "invalid", "kind": "flag", "key": "flag-key", "object": {}}`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})

	t.Run("delete with omitted path", func(t *testing.T) {
		runStreamingTest(t, ldservicesv2.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.Send(httphelpers.SSEEvent{Event: deleteObjectEvent, Data: `{"version": "invalid", "kind": "flag", "key": "flag-key"}`})
			expectRestart(t, p)
			p.mockLog.AssertMessageMatch(t, true, ldlog.Error, ".*malformed JSON data.*will restart")
		})
	})
}

func TestStreamProcessorUnrecoverableErrorsCauseStreamShutdown(t *testing.T) {
	for _, status := range []int{401, 403, 404} {
		t.Run(fmt.Sprintf("HTTP status %d", status), func(t *testing.T) {
			testStreamProcessorUnrecoverableHTTPError(t, status)
		})
	}
}

func runStreamingTest(
	t *testing.T,
	initialData *ldservicesv2.ServerSDKData,
	test func(streamingTestParams),
) {
	runStreamingTestWithConfiguration(t, initialData, nil, test)
}

func runStreamingTestWithConfiguration(
	t *testing.T,
	initialData *ldservicesv2.ServerSDKData,
	configureUpdates func(*mocks.MockDataSourceUpdates),
	test func(streamingTestParams),
) {
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(initialData.ToPutObjects()).
		WithTransferred(1)

	events := make(chan eventsource.Event, 1000)
	streamHandler, stream := ldservices.ServerSideStreamingServiceHandler(protocol.Next())
	protocol.Enqueue(stream)

	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)

	headers := make(http.Header)
	headers.Set(streamProcessorTestHeaderName, streamProcessorTestHeaderValue)
	mockLog := ldlogtest.NewMockLog()
	mockLog.Loggers.SetMinLevel(ldlog.Debug)
	defer mockLog.DumpIfTestFailed(t)
	context := sharedtest.NewTestContext("", &subsystems.HTTPConfiguration{DefaultHeaders: headers},
		&subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})

	httphelpers.WithServer(
		handler,
		func(streamServer *httptest.Server) {
			dataSourceUpdates := mocks.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
			if configureUpdates != nil {
				configureUpdates(dataSourceUpdates)
			}

			dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
			statusReporter := mocks.NewMockStatusReporter()

			sp := NewStreamProcessor(
				context,
				dd,
				statusReporter,
				datasource.StreamConfig{
					URI:                   streamServer.URL,
					InitialReconnectDelay: briefDelay,
				},
			)
			defer sp.Close()

			closeWhenReady := make(chan struct{})

			sp.Sync(closeWhenReady)

			if !th.AssertChannelClosed(t, closeWhenReady, time.Second, "timed out waiting for data source to start") {
				return
			}

			params := streamingTestParams{
				events:         events,
				statusReporter: statusReporter,
				stream:         stream,
				requests:       requestsCh,
				mockLog:        mockLog,
			}

			test(params)
		},
	)
}

func testStreamProcessorRecoverableHTTPError(t *testing.T, statusCode int) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)
	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	sequentialHandler := httphelpers.SequentialHandler(
		httphelpers.HandlerWithStatus(statusCode), // fails the first time
		streamHandler, // then gets a valid stream
	)
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	httphelpers.WithServer(sequentialHandler, func(ts *httptest.Server) {
		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		statusReporter := mocks.NewMockStatusReporter()

		id := ldevents.NewDiagnosticID(sharedtest.TestSDKKey)
		diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
		context := &internal.ClientContextImpl{
			BasicClientContext: subsystems.BasicClientContext{
				SDKKey:  sharedtest.TestSDKKey,
				Logging: subsystems.LoggingConfiguration{Loggers: mockLog.Loggers},
			},
			DiagnosticsManager: diagnosticsManager,
		}

		sp := NewStreamProcessor(context, dd, statusReporter, datasource.StreamConfig{URI: ts.URL, InitialReconnectDelay: briefDelay})
		defer sp.Close()

		closeWhenReady := make(chan struct{})
		sp.Sync(closeWhenReady)

		th.AssertChannelClosed(t, closeWhenReady, time.Second*3, "Should have successfully retried before now")

		event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
		assert.Equal(t, 2, event.GetByKey("streamInits").Count())
		assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))
		assert.Equal(t, ldvalue.Bool(false), event.GetByKey("streamInits").GetByIndex(1).GetByKey("failed"))

		// should have gotten two status updates: first for the error, then the success - note that we're checking
		// here for Interrupted because that's how the StreamProcessor reports the error, even though in the public
		// API it would show up as Initializing because it was still initializing
		status1 := statusReporter.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
		assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, status1.LastError.Kind)
		assert.Equal(t, statusCode, status1.LastError.StatusCode)
		_ = statusReporter.RequireStatusOf(t, interfaces.DataSourceStateValid)
	})
}

func testStreamProcessorUnrecoverableHTTPError(t *testing.T, statusCode int) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	httphelpers.WithServer(httphelpers.HandlerWithStatus(statusCode), func(ts *httptest.Server) {
		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		statusReporter := mocks.NewMockStatusReporter()

		id := ldevents.NewDiagnosticID(sharedtest.TestSDKKey)
		diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
		context := &internal.ClientContextImpl{
			BasicClientContext: subsystems.BasicClientContext{
				SDKKey:  sharedtest.TestSDKKey,
				Logging: subsystems.LoggingConfiguration{Loggers: mockLog.Loggers},
			},
			DiagnosticsManager: diagnosticsManager,
		}

		sp := NewStreamProcessor(context, dd, statusReporter, datasource.StreamConfig{URI: ts.URL, InitialReconnectDelay: time.Second})
		defer sp.Close()

		closeWhenReady := make(chan struct{})

		sp.Sync(closeWhenReady)

		th.AssertChannelClosed(t, closeWhenReady, time.Second*3, "Initialization shouldn't block after this error")

		event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
		assert.Equal(t, 1, event.GetByKey("streamInits").Count())
		assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))

		status := statusReporter.RequireStatusOf(t, interfaces.DataSourceStateOff)
		assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, status.LastError.Kind)
		assert.Equal(t, statusCode, status.LastError.StatusCode)
	})
}

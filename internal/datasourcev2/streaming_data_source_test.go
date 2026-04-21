package datasourcev2

import (
	"context"
	"encoding/json"
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
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
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
	events     chan<- eventsource.Event
	protocol   *ldservicesv2.StreamingProtocol
	resultChan <-chan subsystems.DataSynchronizerResult
	stream     httphelpers.SSEStreamControl
	requests   <-chan httphelpers.HTTPRequestInfo
	mockLog    *ldlogtest.MockLog
}

func TestStreamingDoesNotWorkAsInitializer(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())

	sp := NewStreamProcessor(
		sharedtest.BasicClientContext(),
		datasource.StreamConfig{
			URI:                   "http://example.com/stream",
			InitialReconnectDelay: time.Millisecond * 50,
		},
	)

	defer sp.Close()
	basis, fallback, err := sp.Fetch(ds, context.Background())
	assert.Nil(t, basis)
	assert.False(t, fallback)
	assert.NotNil(t, err)
}

func TestMalformedStreamBaseURI(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())

	sp := NewStreamProcessor(
		sharedtest.BasicClientContext(),
		datasource.StreamConfig{
			URI:                   ":/",
			InitialReconnectDelay: time.Millisecond * 50,
		},
	)

	defer sp.Close()
	resultChan := sp.Sync(ds)

	result := <-resultChan
	assert.Equal(t, interfaces.DataSourceStateOff, result.State)
	assert.Equal(t, interfaces.DataSourceErrorKindUnknown, result.Error.Kind)
}

func TestStreamingProcessorAppendsFilterParameter(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())

	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
				FilterKey:             "filter-value",
			},
		)

		defer sp.Close()
		sp.Sync(ds)

		r := <-requestsCh

		assert.Equal(t, "filter-value", r.Request.URL.Query().Get("filter"))
	})
}

func TestStreamingProcessorAppendsBasisParameter(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NewSelector("test-state", 1))
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
			},
		)

		defer sp.Close()
		sp.Sync(ds)

		r := <-requestsCh

		assert.Equal(t, "test-state", r.Request.URL.Query().Get("basis"))
	})
}

func TestStreamingProcessorCanMakeSuccessfulRequest(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "something-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("updated-state", 2)
	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	handler, _ := httphelpers.RecordingHandler(streamHandler) // we don't care about getting valid stream data
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
			},
		)

		defer sp.Close()
		resultChan := sp.Sync(ds)
		result := <-resultChan

		assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		assert.NotNil(t, result.ChangeSet)
		assert.Len(t, result.ChangeSet.Changes(), 1)
	})
}

func TestStreamingProcessorHandlesFallbackToFDv1(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())

	header := http.Header{
		"X-LD-FD-Fallback": []string{"true"},
	}
	handler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(500, header, nil))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
			},
		)

		defer sp.Close()
		resultChan := sp.Sync(ds)
		result := <-resultChan

		assert.Equal(t, result.State, interfaces.DataSourceStateOff)
		assert.Equal(t, result.Error.Kind, interfaces.DataSourceErrorKindErrorResponse)
		assert.True(t, result.FallbackToFDv1)
	})
}

func TestStreamingProcessorHandlesFallbackOnSuccessfulResponse(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "something-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("updated-state", 2)
	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	// Wrap the valid SSE handler so the response carries x-ld-fd-fallback: true.
	fallbackOnSuccess := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-LD-FD-Fallback", "true")
		streamHandler.ServeHTTP(w, r)
	})

	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	httphelpers.WithServer(fallbackOnSuccess, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
			},
		)

		defer sp.Close()
		resultChan := sp.Sync(ds)

		// A single Valid result carries both the payload and the FallbackToFDv1 signal — the
		// consumer applies the ChangeSet first, then switches to the FDv1 synchronizer.
		result := <-resultChan
		assert.Equal(t, interfaces.DataSourceStateValid, result.State)
		assert.NotNil(t, result.ChangeSet)
		assert.Len(t, result.ChangeSet.Changes(), 1)
		assert.True(t, result.FallbackToFDv1)
	})
}

func TestStreamingProcessorPreClosingShouldShutdownImmediately(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	handler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
			},
		)

		sp.Close()
		resultChan := sp.Sync(ds)

		// Assert the channel is closed
		th.AssertChannelClosed(t, resultChan, time.Second, "starting a closed processor should not yield results")
	})
}

func TestStreamingProcessorClosingClosesResultChan(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	handler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		sp := NewStreamProcessor(
			sharedtest.BasicClientContext(),
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
			},
		)

		resultChan := sp.Sync(ds)
		sp.Close()

		<-resultChan // Ignore the first close when the error handler deals with the 401
		<-resultChan // Ignore the redundant off returned because the SSE client failed to start

		// Assert the channel is closed
		th.AssertChannelClosed(t, resultChan, time.Second, "starting a closed processor should not yield results")
	})
}

func TestStreamingProcessorDoesNotUseConfiguredTimeoutAsReadTimeout(t *testing.T) {
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(ldservicesv2.NewServerSDKData().ToPutObjects()).
		WithTransferred("state", 1)
	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	ds := mocks.NewMockDataSelector(subsystems.NoSelector())

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
			datasource.StreamConfig{
				URI:                   ts.URL,
				InitialReconnectDelay: time.Millisecond * 50,
			},
		)

		defer sp.Close()
		sp.Sync(ds)

		<-time.After(500 * time.Millisecond)
		assert.Equal(t, 1, len(requestsCh))
	})
}

func TestStreamProcessorRecoverableErrorsCauseStreamRestart(t *testing.T) {
	t.Parallel()

	expectRestart := func(t *testing.T, p streamingTestParams) {
		// Allow time for a reconnect (which sends the initial payload (server-intent), then we can queue up the transferred event.
		<-time.After(300 * time.Millisecond)
		p.protocol.WithIntent(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID: "fake-id", Target: 0, Code: subsystems.IntentNone, Reason: "caughtup",
			},
		}).
			WithTransferred("state", 1).
			Enqueue(p.stream)

		<-p.requests
		th.RequireValue(t, p.requests, time.Millisecond*300, "expected stream restart, did not see one")

		timer := time.NewTimer(time.Millisecond * 100)
		defer timer.Stop()

		isValid := true
	L:
		for {
			select {
			case <-timer.C:
				assert.Fail(t, "timed out waiting for stream to restart")
				return
			case status := <-p.resultChan:
				if isValid && status.State == interfaces.DataSourceStateInterrupted {
					isValid = false
				} else if !isValid && status.State == interfaces.DataSourceStateValid {
					break L
				}
			}
		}
	}

	for _, status := range []int{400, 500} {
		t.Run(fmt.Sprintf("HTTP status %d", status), func(t *testing.T) {
			testStreamProcessorRecoverableHTTPError(t, status)
		})
	}

	t.Run("dropped connection", func(t *testing.T) {
		runStreamingTest(t, ldservicesv2.NewServerSDKData(), func(p streamingTestParams) {
			p.stream.EndAll()
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
			<-p.resultChan
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
			p.stream.Send(httphelpers.SSEEvent{
				Event: putObjectEvent,
				Data:  `{"version": "invalid", "kind": "flag", "key": "flag-key", "object": {}}`,
			})
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
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(initialData.ToPutObjects()).
		WithTransferred("state", 1)

	events := make(chan eventsource.Event, 1000)
	streamHandler, stream := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

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
			ds := mocks.NewMockDataSelector(subsystems.NoSelector())

			sp := NewStreamProcessor(
				context,
				datasource.StreamConfig{
					URI:                   streamServer.URL,
					InitialReconnectDelay: briefDelay,
				},
			)
			defer sp.Close()

			resultChan := sp.Sync(ds)

			status := <-resultChan
			assert.Equal(t, interfaces.DataSourceStateValid, status.State)

			params := streamingTestParams{
				events:     events,
				protocol:   protocol,
				resultChan: resultChan,
				stream:     stream,
				requests:   requestsCh,
				mockLog:    mockLog,
			}

			test(params)
		},
	)
}

func testStreamProcessorRecoverableHTTPError(t *testing.T, statusCode int) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)
	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	sequentialHandler := httphelpers.SequentialHandler(
		httphelpers.HandlerWithStatus(statusCode), // fails the first time
		streamHandler, // then gets a valid stream
	)
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	httphelpers.WithServer(sequentialHandler, func(ts *httptest.Server) {
		ds := mocks.NewMockDataSelector(subsystems.NoSelector())

		id := ldevents.NewDiagnosticID(sharedtest.TestSDKKey)
		diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
		context := &internal.ClientContextImpl{
			BasicClientContext: subsystems.BasicClientContext{
				SDKKey:  sharedtest.TestSDKKey,
				Logging: subsystems.LoggingConfiguration{Loggers: mockLog.Loggers},
			},
			DiagnosticsManager: diagnosticsManager,
		}

		sp := NewStreamProcessor(context, datasource.StreamConfig{URI: ts.URL, InitialReconnectDelay: briefDelay})
		defer sp.Close()

		// should have gotten two status updates: first for the error, then the success - note that we're checking
		// here for Interrupted because that's how the StreamProcessor reports the error, even though in the public
		// API it would show up as Initializing because it was still initializing
		resultChan := sp.Sync(ds)
		result := <-resultChan

		assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)
		assert.Equal(t, statusCode, result.Error.StatusCode)

		result = <-resultChan
		assert.Equal(t, interfaces.DataSourceStateValid, result.State)

		event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
		assert.Equal(t, 2, event.GetByKey("streamInits").Count())
		assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))
		assert.Equal(t, ldvalue.Bool(false), event.GetByKey("streamInits").GetByIndex(1).GetByKey("failed"))
	})
}

func testStreamProcessorUnrecoverableHTTPError(t *testing.T, statusCode int) {
	mockLog := ldlogtest.NewMockLog()
	defer mockLog.DumpIfTestFailed(t)
	httphelpers.WithServer(httphelpers.HandlerWithStatus(statusCode), func(ts *httptest.Server) {
		ds := mocks.NewMockDataSelector(subsystems.NoSelector())

		id := ldevents.NewDiagnosticID(sharedtest.TestSDKKey)
		diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
		context := &internal.ClientContextImpl{
			BasicClientContext: subsystems.BasicClientContext{
				SDKKey:  sharedtest.TestSDKKey,
				Logging: subsystems.LoggingConfiguration{Loggers: mockLog.Loggers},
			},
			DiagnosticsManager: diagnosticsManager,
		}

		sp := NewStreamProcessor(context, datasource.StreamConfig{URI: ts.URL, InitialReconnectDelay: time.Second})
		defer sp.Close()

		resultChan := sp.Sync(ds)
		result := <-resultChan

		event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
		assert.Equal(t, 1, event.GetByKey("streamInits").Count())
		assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))

		assert.Equal(t, interfaces.DataSourceStateOff, result.State)
		assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, result.Error.Kind)
		assert.Equal(t, statusCode, result.Error.StatusCode)
	})
}

func TestStreamingDataSourceHandlesUpToDateAndSubsequentChanges(t *testing.T) {
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "up-to-date",
				Code:   subsystems.IntentNone, // Indicates the data source is up to date
				Reason: "initial-state",
			},
		})

	streamHandler, stream := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	// Set up the mock data destination and server
	dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	handler, _ := httphelpers.RecordingHandler(streamHandler)

	mockLog := ldlogtest.NewMockLog()
	mockLog.Loggers.SetMinLevel(ldlog.Debug)
	defer mockLog.DumpIfTestFailed(t)

	httphelpers.WithServer(
		handler,
		func(streamServer *httptest.Server) {
			httpClientFactory := func() *http.Client {
				c := *http.DefaultClient
				c.Timeout = 200 * time.Millisecond
				return &c
			}
			httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
			context := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

			sp := NewStreamProcessor(context, datasource.StreamConfig{URI: streamServer.URL})
			defer sp.Close()

			// Start the stream and verify the data source is initialized, but the flag doesn't exist.
			resultChan := sp.Sync(dd)
			status := <-resultChan
			assert.Equal(t, interfaces.DataSourceStateValid, status.State)
			assert.Nil(t, status.ChangeSet)

			flag, err := dd.DataStore.Get(datakinds.Features, "test-flag")
			assert.NoError(t, err)
			assert.Nil(t, flag.Item)

			// Push a change through the data source
			change := subsystems.PutObject{
				Kind:    "flag",
				Key:     "test-flag",
				Version: 1,
				Object:  json.RawMessage(`{"key": "test-flag", "version": 1}`),
			}
			protocol.WithPutObjects([]subsystems.PutObject{change})
			protocol.WithTransferred("state", 1)
			protocol.Enqueue(stream)

			// Verify the change is successfully received and applied
			status = <-resultChan
			assert.Equal(t, interfaces.DataSourceStateValid, status.State)
			assert.NotNil(t, status.ChangeSet)
			dd.Apply(*status.ChangeSet, true)

			flag, err = dd.DataStore.Get(datakinds.Features, "test-flag")
			assert.NoError(t, err)
			assert.Equal(t, 1, flag.Version)
			assert.NotNil(t, 1, flag.Item)
		},
	)
}

func TestStreamingDataSourceHandlesResettingFromError(t *testing.T) {
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "payload-id",
				Code:   subsystems.IntentNone, // Indicates the data source is up to date
				Reason: "up-to-date",
			},
		})

	streamHandler, stream := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	// Set up the mock data destination and server
	dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	handler, _ := httphelpers.RecordingHandler(streamHandler)

	mockLog := ldlogtest.NewMockLog()
	mockLog.Loggers.SetMinLevel(ldlog.Debug)
	defer mockLog.DumpIfTestFailed(t)

	httphelpers.WithServer(
		handler,
		func(streamServer *httptest.Server) {
			httpClientFactory := func() *http.Client {
				c := *http.DefaultClient
				c.Timeout = 200 * time.Millisecond
				return &c
			}
			httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
			context := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

			sp := NewStreamProcessor(context, datasource.StreamConfig{URI: streamServer.URL})
			defer sp.Close()

			// Start the stream and verify the data source is initialized, but the flag doesn't exist.
			resultChan := sp.Sync(dd)
			status := <-resultChan
			assert.Equal(t, interfaces.DataSourceStateValid, status.State)
			assert.Nil(t, status.ChangeSet)

			flag, err := dd.DataStore.Get(datakinds.Features, "test-flag")
			assert.NoError(t, err)
			assert.Nil(t, flag.Item)

			// Push through a put for a new flag that will be discarded due to an error
			change := subsystems.PutObject{
				Kind:    "flag",
				Key:     "error-flag",
				Version: 1,
				Object:  json.RawMessage(`{"key": "error-flag", "version": 1}`),
			}
			protocol.WithPutObjects([]subsystems.PutObject{change})

			protocol.WithError(subsystems.Error{
				PayloadID: "payload-id",
				Reason:    "testing failure",
			})

			// Push a change through the data source
			change = subsystems.PutObject{
				Kind:    "flag",
				Key:     "test-flag",
				Version: 1,
				Object:  json.RawMessage(`{"key": "test-flag", "version": 1}`),
			}
			protocol.WithPutObjects([]subsystems.PutObject{change})
			protocol.WithTransferred("state", 1)
			protocol.Enqueue(stream)

			// Verify the change is successfully received and applied
			status = <-resultChan
			assert.Equal(t, interfaces.DataSourceStateValid, status.State)
			assert.NotNil(t, status.ChangeSet)
			dd.Apply(*status.ChangeSet, true)

			flag, err = dd.DataStore.Get(datakinds.Features, "test-flag")
			assert.NoError(t, err)
			assert.Equal(t, 1, flag.Version)
			assert.NotNil(t, 1, flag.Item)

			flag, err = dd.DataStore.Get(datakinds.Features, "error-flag")
			assert.NoError(t, err)
			assert.Nil(t, flag.Item)
		},
	)
}

func TestStreamingDataSourceIgnoresGoodbye(t *testing.T) {
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "payload-id",
				Code:   subsystems.IntentNone, // Indicates the data source is up to date
				Reason: "up-to-date",
			},
		})

	streamHandler, stream := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	// Set up the mock data destination and server
	dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	handler, _ := httphelpers.RecordingHandler(streamHandler)

	mockLog := ldlogtest.NewMockLog()
	mockLog.Loggers.SetMinLevel(ldlog.Debug)
	defer mockLog.DumpIfTestFailed(t)

	httphelpers.WithServer(
		handler,
		func(streamServer *httptest.Server) {
			httpClientFactory := func() *http.Client {
				c := *http.DefaultClient
				c.Timeout = 200 * time.Millisecond
				return &c
			}
			httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
			context := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

			sp := NewStreamProcessor(context, datasource.StreamConfig{URI: streamServer.URL})
			defer sp.Close()

			// Start the stream and verify the data source is initialized, but the flag doesn't exist.
			resultChan := sp.Sync(dd)
			status := <-resultChan
			assert.Equal(t, interfaces.DataSourceStateValid, status.State)
			assert.Nil(t, status.ChangeSet)

			flag, err := dd.DataStore.Get(datakinds.Features, "test-flag")
			assert.NoError(t, err)
			assert.Nil(t, flag.Item)

			// Push through a put for a new flag that will be discarded due to an error
			change := subsystems.PutObject{
				Kind:    "flag",
				Key:     "before-goodbye-flag",
				Version: 1,
				Object:  json.RawMessage(`{"key": "before-goodbye-flag", "version": 1}`),
			}
			protocol.WithPutObjects([]subsystems.PutObject{change})

			protocol.WithGoodbye(subsystems.Goodbye{
				Reason:      "for testing reason",
				Silent:      false,
				Catastrophe: false,
			})

			// Push a change through the data source
			change = subsystems.PutObject{
				Kind:    "flag",
				Key:     "test-flag",
				Version: 1,
				Object:  json.RawMessage(`{"key": "test-flag", "version": 1}`),
			}
			protocol.WithPutObjects([]subsystems.PutObject{change})
			protocol.WithTransferred("state", 1)
			protocol.Enqueue(stream)

			// Verify the change is successfully received and applied
			status = <-resultChan
			assert.Equal(t, interfaces.DataSourceStateValid, status.State)
			assert.NotNil(t, status.ChangeSet)
			dd.Apply(*status.ChangeSet, true)

			flag, err = dd.DataStore.Get(datakinds.Features, "test-flag")
			assert.NoError(t, err)
			assert.Equal(t, 1, flag.Version)
			assert.NotNil(t, 1, flag.Item)

			flag, err = dd.DataStore.Get(datakinds.Features, "before-goodbye-flag")
			assert.NoError(t, err)
			assert.Equal(t, 1, flag.Version)
			assert.NotNil(t, 1, flag.Item)
		},
	)
}

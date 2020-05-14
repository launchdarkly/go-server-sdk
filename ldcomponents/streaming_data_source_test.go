package ldcomponents

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/httphelpers"
	"github.com/launchdarkly/go-test-helpers/ldservices"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

	"github.com/launchdarkly/eventsource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const briefDelay = time.Millisecond * 50

func runStreamingTest(
	t *testing.T,
	initialEvent eventsource.Event,
	test func(events chan<- eventsource.Event, p dataSourceTestParams),
) {
	events := make(chan eventsource.Event, 1000)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialEvent, events)
	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		flagEndpointHandler := httphelpers.HandlerForPath(
			"/sdk/latest-flags/my-flag",
			httphelpers.HandlerWithJSONResponse(ldservices.FlagOrSegment("my-flag", 5), nil),
			nil,
		)
		httphelpers.WithServer(flagEndpointHandler, func(sdkServer *httptest.Server) {
			withDataSourceTestParams(func(params dataSourceTestParams) {
				sp, err := StreamingDataSource().
					BaseURI(streamServer.URL).
					PollingBaseURI(sdkServer.URL).
					InitialReconnectDelay(briefDelay).
					CreateDataSource(basicClientContext(), params.store, params.dataStoreStatusProvider)
				require.NoError(t, err)
				defer sp.Close()

				closeWhenReady := make(chan struct{})

				sp.Start(closeWhenReady)

				select {
				case <-closeWhenReady:
				case <-time.After(time.Second):
					assert.Fail(t, "start timeout")
					return
				}

				test(events, params)
			})
		})
	})
}

func TestStreamProcessor(t *testing.T) {
	t.Parallel()
	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2)).Segments(ldservices.FlagOrSegment("my-segment", 2))

	t.Run("initial put", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, p dataSourceTestParams) {
			p.waitForInit(t, initialData)
		})
	})

	t.Run("patch flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, p dataSourceTestParams) {
			events <- ldservices.NewSSEEvent("", patchEvent, `{"path": "/flags/my-flag", "data": {"key": "my-flag", "version": 3}}`)

			p.waitForUpdate(t, interfaces.DataKindFeatures(), "my-flag", 3)
		})
	})

	t.Run("delete flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, p dataSourceTestParams) {
			events <- ldservices.NewSSEEvent("", deleteEvent, `{"path": "/flags/my-flag", "version": 4}`)

			p.waitForDelete(t, interfaces.DataKindSegments(), "my-flag", 4)
		})
	})

	t.Run("patch segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, p dataSourceTestParams) {
			events <- ldservices.NewSSEEvent("", patchEvent, `{"path": "/segments/my-segment", "data": {"key": "my-segment", "version": 7}}`)

			p.waitForUpdate(t, interfaces.DataKindSegments(), "my-segment", 7)
		})
	})

	t.Run("delete segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, p dataSourceTestParams) {
			events <- ldservices.NewSSEEvent("", deleteEvent, `{"path": "/segments/my-segment", "version": 8}`)

			p.waitForDelete(t, interfaces.DataKindSegments(), "my-segment", 8)
		})
	})

	t.Run("indirect flag patch", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, p dataSourceTestParams) {
			events <- ldservices.NewSSEEvent("", indirectPatchEvent, "/flags/my-flag")

			p.waitForUpdate(t, interfaces.DataKindFeatures(), "my-flag", 5)
		})
	})
}

func TestStreamProcessorDoesNotFailImmediatelyOn400(t *testing.T) {
	testStreamProcessorRecoverableError(t, 400)
}

func TestStreamProcessorFailsImmediatelyOn401(t *testing.T) {
	testStreamProcessorUnrecoverableError(t, 401)
}

func TestStreamProcessorFailsImmediatelyOn403(t *testing.T) {
	testStreamProcessorUnrecoverableError(t, 403)
}

func TestStreamProcessorDoesNotFailImmediatelyOn500(t *testing.T) {
	testStreamProcessorRecoverableError(t, 500)
}

func testStreamProcessorUnrecoverableError(t *testing.T, statusCode int) {
	httphelpers.WithServer(httphelpers.HandlerWithStatus(statusCode), func(ts *httptest.Server) {
		withDataSourceTestParams(func(p dataSourceTestParams) {
			id := ldevents.NewDiagnosticID(testSdkKey)
			diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
			context := newClientContextWithDiagnostics(testSdkKey, nil, nil, diagnosticsManager)

			sp, err := StreamingDataSource().BaseURI(ts.URL).
				CreateDataSource(context, p.store, p.dataStoreStatusProvider)
			require.NoError(t, err)
			defer sp.Close()

			closeWhenReady := make(chan struct{})

			sp.Start(closeWhenReady)

			select {
			case <-closeWhenReady:
				assert.False(t, sp.Initialized())
			case <-time.After(time.Second * 3):
				assert.Fail(t, "Initialization shouldn't block after this error")
			}

			event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
			assert.Equal(t, 1, event.GetByKey("streamInits").Count())
			assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))
		})
	})
}

func testStreamProcessorRecoverableError(t *testing.T, statusCode int) {
	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData, nil)
	sequentialHandler := httphelpers.SequentialHandler(
		httphelpers.HandlerWithStatus(statusCode), // fails the first time
		streamHandler, // then gets a valid stream
	)
	httphelpers.WithServer(sequentialHandler, func(ts *httptest.Server) {
		withDataSourceTestParams(func(p dataSourceTestParams) {
			id := ldevents.NewDiagnosticID(testSdkKey)
			diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
			context := newClientContextWithDiagnostics(testSdkKey, nil, nil, diagnosticsManager)

			sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
				CreateDataSource(context, p.store, p.dataStoreStatusProvider)
			require.NoError(t, err)
			defer sp.Close()

			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			select {
			case <-closeWhenReady:
				assert.True(t, sp.Initialized())
			case <-time.After(time.Second * 3):
				assert.Fail(t, "Should have successfully retried before now")
			}

			event := diagnosticsManager.CreateStatsEventAndReset(0, 0, 0)
			assert.Equal(t, 2, event.GetByKey("streamInits").Count())
			assert.Equal(t, ldvalue.Bool(true), event.GetByKey("streamInits").GetByIndex(0).GetByKey("failed"))
			assert.Equal(t, ldvalue.Bool(false), event.GetByKey("streamInits").GetByIndex(1).GetByKey("failed"))
		})
	})
}

func TestStreamProcessorUsesHTTPClientFactory(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		withDataSourceTestParams(func(p dataSourceTestParams) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			context := interfaces.NewClientContext(testSdkKey, nil, httpClientFactory, sharedtest.NewTestLoggers)

			sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
				CreateDataSource(context, p.store, p.dataStoreStatusProvider)
			require.NoError(t, err)
			defer sp.Close()
			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			r := <-requestsCh

			assert.Equal(t, "/all/transformed", r.Request.URL.Path)
		})
	})
}

func TestStreamProcessorDoesNotUseConfiguredTimeoutAsReadTimeout(t *testing.T) {
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(ldservices.NewServerSDKData(), nil)
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		withDataSourceTestParams(func(p dataSourceTestParams) {
			httpClientFactory := func() *http.Client {
				c := *http.DefaultClient
				c.Timeout = 200 * time.Millisecond
				return &c
			}
			context := interfaces.NewClientContext(testSdkKey, nil, httpClientFactory, sharedtest.NewTestLoggers())

			sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
				CreateDataSource(context, p.store, p.dataStoreStatusProvider)
			require.NoError(t, err)
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
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData, nil)

	httphelpers.WithServer(streamHandler, func(ts *httptest.Server) {
		withDataSourceTestParams(func(p dataSourceTestParams) {
			sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
				CreateDataSource(basicClientContext(), p.store, p.dataStoreStatusProvider)
			require.NoError(t, err)
			defer sp.Close()

			closeWhenReady := make(chan struct{})
			sp.Start(closeWhenReady)

			// Wait until the stream has received data and put it in the store
			p.waitForInit(t, initialData)

			// Change the stream's initialData so we'll get different data the next time it restarts
			initialData.Flags(ldservices.FlagOrSegment("my-flag", 2))

			// Make the data store simulate an outage and recovery with NeedsRefresh: true
			p.dataStoreUpdates.UpdateStatus(interfaces.DataStoreStatus{Available: false})
			p.dataStoreUpdates.UpdateStatus(interfaces.DataStoreStatus{Available: true, NeedsRefresh: true})

			// When the stream restarts, it'll call Init with the refreshed data
			p.waitForInit(t, initialData)
		})
	})
}

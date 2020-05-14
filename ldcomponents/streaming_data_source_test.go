package ldcomponents

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/httphelpers"
	"github.com/launchdarkly/go-test-helpers/ldservices"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/launchdarkly/eventsource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

const briefDelay = time.Millisecond * 50

func runStreamingTest(t *testing.T, initialEvent eventsource.Event, test func(events chan<- eventsource.Event, store interfaces.DataStore)) {
	events := make(chan eventsource.Event, 1000)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialEvent, events)
	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		flagEndpointHandler := httphelpers.HandlerForPath(
			"/sdk/latest-flags/my-flag",
			httphelpers.HandlerWithJSONResponse(ldservices.FlagOrSegment("my-flag", 5), nil),
			nil,
		)
		httphelpers.WithServer(flagEndpointHandler, func(sdkServer *httptest.Server) {
			store := makeInMemoryDataStore()
			sp, err := StreamingDataSource().BaseURI(streamServer.URL).PollingBaseURI(sdkServer.URL).InitialReconnectDelay(briefDelay).
				CreateDataSource(basicClientContext(), store)
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

			test(events, store)
		})
	})
}

func TestStreamProcessor(t *testing.T) {
	t.Parallel()
	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2)).Segments(ldservices.FlagOrSegment("my-segment", 2))

	t.Run("initial put", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store interfaces.DataStore) {
			waitForVersion(t, store, interfaces.DataKindFeatures(), "my-flag", 2)
		})
	})

	t.Run("patch flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store interfaces.DataStore) {
			events <- ldservices.NewSSEEvent("", patchEvent, `{"path": "/flags/my-flag", "data": {"key": "my-flag", "version": 3}}`)

			waitForVersion(t, store, interfaces.DataKindFeatures(), "my-flag", 3)
		})
	})

	t.Run("delete flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store interfaces.DataStore) {
			events <- ldservices.NewSSEEvent("", deleteEvent, `{"path": "/flags/my-flag", "version": 4}`)

			waitForDelete(t, store, interfaces.DataKindSegments(), "my-flag")
		})
	})

	t.Run("patch segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store interfaces.DataStore) {
			events <- ldservices.NewSSEEvent("", patchEvent, `{"path": "/segments/my-segment", "data": {"key": "my-segment", "version": 7}}`)

			waitForVersion(t, store, interfaces.DataKindSegments(), "my-segment", 7)
		})
	})

	t.Run("delete segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store interfaces.DataStore) {
			events <- ldservices.NewSSEEvent("", deleteEvent, `{"path": "/segments/my-segment", "version": 8}`)

			waitForDelete(t, store, interfaces.DataKindSegments(), "my-segment")
		})
	})

	t.Run("indirect flag patch", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store interfaces.DataStore) {
			events <- ldservices.NewSSEEvent("", indirectPatchEvent, "/flags/my-flag")

			waitForVersion(t, store, interfaces.DataKindFeatures(), "my-flag", 5)
		})
	})
}

func waitForVersion(t *testing.T, store interfaces.DataStore, kind interfaces.VersionedDataKind, key string, version int) interfaces.VersionedData {
	var item interfaces.VersionedData
	var err error
	deadline := time.Now().Add(time.Second * 3)
	for {
		item, err = store.Get(kind, key)
		if err == nil && item != nil && item.GetVersion() == version || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if assert.NoError(t, err) && assert.NotNil(t, item) && assert.Equal(t, version, item.GetVersion()) {
		return item
	}
	return nil
}

func waitForDelete(t *testing.T, store interfaces.DataStore, kind interfaces.VersionedDataKind, key string) {
	var item interfaces.VersionedData
	var err error
	deadline := time.Now().Add(time.Second * 3)
	for {
		item, err = store.Get(kind, key)
		if item == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	assert.NoError(t, err)
	assert.Nil(t, item)
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
		id := ldevents.NewDiagnosticID(testSdkKey)
		diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
		store := makeInMemoryDataStore()
		context := newClientContextWithDiagnostics(testSdkKey, nil, nil, diagnosticsManager)

		sp, err := StreamingDataSource().BaseURI(ts.URL).
			CreateDataSource(context, store)
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
}

func testStreamProcessorRecoverableError(t *testing.T, statusCode int) {
	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData, nil)
	sequentialHandler := httphelpers.SequentialHandler(
		httphelpers.HandlerWithStatus(statusCode), // fails the first time
		streamHandler, // then gets a valid stream
	)
	httphelpers.WithServer(sequentialHandler, func(ts *httptest.Server) {
		id := ldevents.NewDiagnosticID(testSdkKey)
		diagnosticsManager := ldevents.NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
		store := makeInMemoryDataStore()
		context := newClientContextWithDiagnostics(testSdkKey, nil, nil, diagnosticsManager)

		sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
			CreateDataSource(context, store)
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
}

func TestStreamProcessorUsesHTTPClientFactory(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
		store := makeInMemoryDataStore()
		context := interfaces.NewClientContext(testSdkKey, nil, httpClientFactory, ldlog.NewDisabledLoggers())

		sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
			CreateDataSource(context, store)
		require.NoError(t, err)
		defer sp.Close()
		closeWhenReady := make(chan struct{})
		sp.Start(closeWhenReady)

		r := <-requestsCh

		assert.Equal(t, "/all/transformed", r.Request.URL.Path)
	})
}

func TestStreamProcessorDoesNotUseConfiguredTimeoutAsReadTimeout(t *testing.T) {
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(ldservices.NewServerSDKData(), nil)
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		httpClientFactory := func() *http.Client {
			c := *http.DefaultClient
			c.Timeout = 200 * time.Millisecond
			return &c
		}
		store := makeInMemoryDataStore()
		context := interfaces.NewClientContext(testSdkKey, nil, httpClientFactory, ldlog.NewDisabledLoggers())

		sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
			CreateDataSource(context, store)
		require.NoError(t, err)
		defer sp.Close()
		closeWhenReady := make(chan struct{})
		sp.Start(closeWhenReady)

		<-time.After(500 * time.Millisecond)
		assert.Equal(t, 1, len(requestsCh))
	})
}

func TestStreamProcessorRestartsStreamIfStoreNeedsRefresh(t *testing.T) {
	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 1))
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData, nil)

	httphelpers.WithServer(streamHandler, func(ts *httptest.Server) {
		store := &testDataStoreWithStatus{
			inits: make(chan map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData),
		}

		sp, err := StreamingDataSource().BaseURI(ts.URL).InitialReconnectDelay(briefDelay).
			CreateDataSource(basicClientContext(), store)
		require.NoError(t, err)
		defer sp.Close()

		closeWhenReady := make(chan struct{})
		sp.Start(closeWhenReady)

		// Wait until the stream has received data and put it in the store
		receivedInitialData := <-store.inits
		assert.Equal(t, 1, receivedInitialData[interfaces.DataKindFeatures()]["my-flag"].GetVersion())

		// Change the stream's initialData so we'll get different data the next time it restarts
		initialData.Flags(ldservices.FlagOrSegment("my-flag", 2))

		// Make the data store simulate an outage and recovery with NeedsRefresh: true
		store.publishStatus(internal.DataStoreStatus{Available: false})
		store.publishStatus(internal.DataStoreStatus{Available: true, NeedsRefresh: true})

		// When the stream restarts, it'll call Init with the refreshed data
		receivedNewData := <-store.inits
		assert.Equal(t, 2, receivedNewData[interfaces.DataKindFeatures()]["my-flag"].GetVersion())
	})
}

type testDataStoreWithStatus struct {
	inits     chan map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData
	statusSub *testStatusSubscription
}

func (t *testDataStoreWithStatus) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	return nil, nil
}

func (t *testDataStoreWithStatus) All(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	return nil, nil
}

func (t *testDataStoreWithStatus) Init(data map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	t.inits <- data
	return nil
}

func (t *testDataStoreWithStatus) Delete(kind interfaces.VersionedDataKind, key string, version int) error {
	return nil
}

func (t *testDataStoreWithStatus) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) error {
	return nil
}

func (t *testDataStoreWithStatus) Initialized() bool {
	return true
}

func (t *testDataStoreWithStatus) GetStoreStatus() internal.DataStoreStatus {
	return internal.DataStoreStatus{Available: true}
}

func (t *testDataStoreWithStatus) StatusSubscribe() internal.DataStoreStatusSubscription {
	t.statusSub = &testStatusSubscription{
		ch: make(chan internal.DataStoreStatus),
	}
	return t.statusSub
}

func (t *testDataStoreWithStatus) Close() error {
	return nil
}

func (t *testDataStoreWithStatus) publishStatus(status internal.DataStoreStatus) {
	if t.statusSub != nil {
		t.statusSub.ch <- status
	}
}

type testStatusSubscription struct {
	ch chan internal.DataStoreStatus
}

func (s *testStatusSubscription) Channel() <-chan internal.DataStoreStatus {
	return s.ch
}

func (s *testStatusSubscription) Close() {
	close(s.ch)
}

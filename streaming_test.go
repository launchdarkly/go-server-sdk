package ldclient

import (
	"errors"
	"io/ioutil"
	"log"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/eventsource"
	"github.com/launchdarkly/go-test-helpers/httphelpers"
	"github.com/launchdarkly/go-test-helpers/ldservices"
	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-server-sdk.v4/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v4/ldlog"
	shared "gopkg.in/launchdarkly/go-server-sdk.v4/shared_test"
)

func runStreamingTest(t *testing.T, initialEvent eventsource.Event, test func(events chan<- eventsource.Event, store FeatureStore)) {
	events := make(chan eventsource.Event, 1000)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialEvent, events)
	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		flagEndpointHandler := httphelpers.HandlerForPath(
			"/sdk/latest-flags/my-flag",
			httphelpers.HandlerWithJSONResponse(ldservices.FlagOrSegment("my-flag", 5), nil),
			nil,
		)
		httphelpers.WithServer(flagEndpointHandler, func(sdkServer *httptest.Server) {
			store := NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0))

			cfg := Config{
				FeatureStore: store,
				StreamUri:    streamServer.URL,
				BaseUri:      sdkServer.URL,
				Loggers:      ldlog.NewDefaultLoggers(),
			}

			requestor := newRequestor("sdkKey", cfg, nil)
			sp := newStreamProcessor("sdkKey", cfg, requestor)
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
	initialData := ldservices.NewServerSDKData().
		Flags(ldservices.FlagOrSegment("my-flag", 2)).
		Segments(ldservices.FlagOrSegment("my-segment", 2))

	t.Run("initial put", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store FeatureStore) {
			waitForVersion(t, store, Features, "my-flag", 2)
		})
	})

	t.Run("patch flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- ldservices.NewSSEEvent("", patchEvent, `{"path": "/flags/my-flag", "data": {"key": "my-flag", "version": 3}}`)

			waitForVersion(t, store, Features, "my-flag", 3)
		})
	})

	t.Run("delete flag", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- ldservices.NewSSEEvent("", deleteEvent, `{"path": "/flags/my-flag", "version": 4}`)

			waitForDelete(t, store, Segments, "my-flag")
		})
	})

	t.Run("patch segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- ldservices.NewSSEEvent("", patchEvent, `{"path": "/segments/my-segment", "data": {"key": "my-segment", "version": 7}}`)

			waitForVersion(t, store, Segments, "my-segment", 7)
		})
	})

	t.Run("delete segment", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- ldservices.NewSSEEvent("", deleteEvent, `{"path": "/segments/my-segment", "version": 8}`)

			waitForDelete(t, store, Segments, "my-segment")
		})
	})

	t.Run("indirect flag patch", func(t *testing.T) {
		runStreamingTest(t, initialData, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- ldservices.NewSSEEvent("", indirectPatchEvent, "/flags/my-flag")

			waitForVersion(t, store, Features, "my-flag", 5)
		})
	})
}

func waitForVersion(t *testing.T, store FeatureStore, kind VersionedDataKind, key string, version int) VersionedData {
	var item VersionedData
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

func waitForDelete(t *testing.T, store FeatureStore, kind VersionedDataKind, key string) {
	var item VersionedData
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
		id := newDiagnosticId("sdkKey")
		diagnosticsManager := newDiagnosticsManager(id, Config{}, time.Second, time.Now(), nil)
		cfg := Config{
			StreamUri:          ts.URL,
			FeatureStore:       NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0)),
			Loggers:            shared.NullLoggers(),
			diagnosticsManager: diagnosticsManager,
		}

		sp := newStreamProcessor("sdkKey", cfg, nil)
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
		assert.Equal(t, 1, len(event.StreamInits))
		assert.True(t, event.StreamInits[0].Failed)
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
		id := newDiagnosticId("sdkKey")
		diagnosticsManager := newDiagnosticsManager(id, Config{}, time.Second, time.Now(), nil)
		cfg := Config{
			StreamUri:          ts.URL,
			FeatureStore:       NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0)),
			Loggers:            shared.NullLoggers(),
			diagnosticsManager: diagnosticsManager,
		}

		sp := newStreamProcessor("sdkKey", cfg, nil)
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
		if assert.Equal(t, 2, len(event.StreamInits)) {
			assert.True(t, event.StreamInits[0].Failed)
			assert.False(t, event.StreamInits[1].Failed)
		}
	})
}

func TestStreamProcessorUsesHTTPClientFactory(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401)) // we don't care about getting valid stream data

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		cfg := Config{
			Loggers:           shared.NullLoggers(),
			StreamUri:         ts.URL,
			HTTPClientFactory: urlAppendingHTTPClientFactory("/transformed"),
		}

		sp := newStreamProcessor("sdkKey", cfg, nil)
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
		cfg := Config{
			Loggers:   shared.NullLoggers(),
			StreamUri: ts.URL,
			Timeout:   200 * time.Millisecond,
		}

		sp := newStreamProcessor("sdkKey", cfg, nil)
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
		store := &testFeatureStoreWithStatus{
			inits: make(chan map[VersionedDataKind]map[string]VersionedData),
		}
		cfg := Config{
			StreamUri:    ts.URL,
			FeatureStore: store,
			Loggers:      shared.NullLoggers(),
		}
		sp := newStreamProcessor("sdkKey", cfg, nil)
		defer sp.Close()

		closeWhenReady := make(chan struct{})
		sp.Start(closeWhenReady)

		// Wait until the stream has received data and put it in the store
		receivedInitialData := <-store.inits
		assert.Equal(t, 1, receivedInitialData[Features]["my-flag"].GetVersion())

		// Change the stream's initialEvent so we'll get different data the next time it restarts
		initialData.Flags(ldservices.FlagOrSegment("my-flag", 2))

		// Make the feature store simulate an outage and recovery with NeedsRefresh: true
		store.publishStatus(internal.FeatureStoreStatus{Available: false})
		store.publishStatus(internal.FeatureStoreStatus{Available: true, NeedsRefresh: true})

		// When the stream restarts, it'll call Init with the refreshed data
		receivedNewData := <-store.inits
		assert.Equal(t, 2, receivedNewData[Features]["my-flag"].GetVersion())
	})
}

func TestStreamProcessorRestartsStreamIfStoreNeedsRefreshAfterInitFails(t *testing.T) {
	// This is the same as TestStreamProcessorRestartsStreamIfStoreNeedsRefresh except that instead of
	// the store having an outage after a successful initialization, it fails on the initialization itself.
	// We had a bug where if the Init call failed, the stream would give up and never restart.

	initialData := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 1))
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(initialData, nil)
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)

	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		store := &testFeatureStoreWithStatus{
			inits: make(chan map[VersionedDataKind]map[string]VersionedData),
		}
		store.setInitError(errors.New("sorry"))
		cfg := Config{
			StreamUri:    ts.URL,
			FeatureStore: store,
			Loggers:      shared.NullLoggers(),
		}
		sp := newStreamProcessor("sdkKey", cfg, nil)
		defer sp.Close()

		closeWhenReady := make(chan struct{})
		sp.Start(closeWhenReady)

		// Wait until the stream has received data and tried to put it in the store (the store's Init will fail)
		_ = <-requestsCh
		receivedInitialData := <-store.inits
		assert.Equal(t, 1, receivedInitialData[Features]["my-flag"].GetVersion())

		// Change the stream's initialEvent so we'll get different data the next time it restarts
		initialData.Flags(ldservices.FlagOrSegment("my-flag", 2))

		// Make the feature store simulate an outage and recovery with NeedsRefresh: true
		store.setInitError(nil)
		store.publishStatus(internal.FeatureStoreStatus{Available: false})
		store.publishStatus(internal.FeatureStoreStatus{Available: true, NeedsRefresh: true})

		// When the stream restarts, it'll call Init with the refreshed data
		_ = <-requestsCh
		receivedNewData := <-store.inits
		assert.Equal(t, 2, receivedNewData[Features]["my-flag"].GetVersion())
	})
}

type testFeatureStoreWithStatus struct {
	inits     chan map[VersionedDataKind]map[string]VersionedData
	statusSub *testStatusSubscription
	initError error
	lock      sync.Mutex
}

func (t *testFeatureStoreWithStatus) Get(kind VersionedDataKind, key string) (VersionedData, error) {
	return nil, nil
}

func (t *testFeatureStoreWithStatus) All(kind VersionedDataKind) (map[string]VersionedData, error) {
	return nil, nil
}

func (t *testFeatureStoreWithStatus) Init(data map[VersionedDataKind]map[string]VersionedData) error {
	t.inits <- data
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.initError
}

func (t *testFeatureStoreWithStatus) Delete(kind VersionedDataKind, key string, version int) error {
	return nil
}

func (t *testFeatureStoreWithStatus) Upsert(kind VersionedDataKind, item VersionedData) error {
	return nil
}

func (t *testFeatureStoreWithStatus) Initialized() bool {
	return true
}

func (t *testFeatureStoreWithStatus) GetStoreStatus() internal.FeatureStoreStatus {
	return internal.FeatureStoreStatus{Available: true}
}

func (t *testFeatureStoreWithStatus) StatusSubscribe() internal.FeatureStoreStatusSubscription {
	t.statusSub = &testStatusSubscription{
		ch: make(chan internal.FeatureStoreStatus),
	}
	return t.statusSub
}

func (t *testFeatureStoreWithStatus) publishStatus(status internal.FeatureStoreStatus) {
	if t.statusSub != nil {
		t.statusSub.ch <- status
	}
}

func (t *testFeatureStoreWithStatus) setInitError(err error) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.initError = err
}

type testStatusSubscription struct {
	ch chan internal.FeatureStoreStatus
}

func (s *testStatusSubscription) Channel() <-chan internal.FeatureStoreStatus {
	return s.ch
}

func (s *testStatusSubscription) Close() {
	close(s.ch)
}

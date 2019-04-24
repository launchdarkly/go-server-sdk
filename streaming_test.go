package ldclient

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/eventsource"
	"github.com/stretchr/testify/assert"
)

type testEvent struct {
	id, event, data string
}

func (e *testEvent) Id() string    { return e.id }
func (e *testEvent) Event() string { return e.event }
func (e *testEvent) Data() string  { return e.data }

type testRepo struct {
	initialEvent eventsource.Event
}

func (r *testRepo) Replay(channel, id string) chan eventsource.Event {
	c := make(chan eventsource.Event, 1)
	c <- r.initialEvent
	return c
}

func runStreamingTest(t *testing.T, initialEvent eventsource.Event, test func(events chan<- eventsource.Event, store FeatureStore)) {
	esserver := eventsource.NewServer()
	esserver.ReplayAll = true
	esserver.Register("test", &testRepo{initialEvent: initialEvent})
	events := make(chan eventsource.Event, 1000)
	streamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/all", r.URL.Path)
		go func() {
			for e := range events {
				esserver.Publish([]string{"test"}, e)
			}
		}()
		esserver.Handler("test").ServeHTTP(w, r)
	}))
	defer streamServer.Close()

	sdkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sdk/latest-flags/my-flag", r.URL.Path)
		w.Write([]byte(`{"key": "my-flag", "version": 5}`))
	}))
	defer sdkServer.Close()
	defer esserver.Close()

	store := NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0))

	cfg := Config{
		FeatureStore: store,
		StreamUri:    streamServer.URL,
		BaseUri:      sdkServer.URL,
		Logger:       log.New(ioutil.Discard, "", 0),
	}

	requestor := newRequestor("sdkKey", cfg)
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
}

func TestStreamProcessor(t *testing.T) {
	t.Parallel()
	initialPutEvent := &testEvent{
		event: putEvent,
		data: `{"path": "/", "data": {
"flags": {"my-flag": {"key": "my-flag", "version": 2}},
"segments": {"my-segment": {"key": "my-segment", "version": 5}}
}}`,
	}

	t.Run("initial put", func(t *testing.T) {
		runStreamingTest(t, initialPutEvent, func(events chan<- eventsource.Event, store FeatureStore) {
			waitForVersion(t, store, Features, "my-flag", 2)
		})
	})

	t.Run("patch flag", func(t *testing.T) {
		runStreamingTest(t, initialPutEvent, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- &testEvent{
				event: patchEvent,
				data:  `{"path": "/flags/my-flag", "data": {"key": "my-flag", "version": 3}}`,
			}

			waitForVersion(t, store, Features, "my-flag", 3)
		})
	})

	t.Run("delete flag", func(t *testing.T) {
		runStreamingTest(t, initialPutEvent, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- &testEvent{
				event: deleteEvent,
				data:  `{"path": "/flags/my-flag", "version": 4}`,
			}

			waitForDelete(t, store, Segments, "my-flag")
		})
	})

	t.Run("patch segment", func(t *testing.T) {
		runStreamingTest(t, initialPutEvent, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- &testEvent{
				event: patchEvent,
				data:  `{"path": "/segments/my-segment", "data": {"key": "my-segment", "version": 7}}`,
			}

			waitForVersion(t, store, Segments, "my-segment", 7)
		})
	})

	t.Run("delete segment", func(t *testing.T) {
		runStreamingTest(t, initialPutEvent, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- &testEvent{
				event: deleteEvent,
				data:  `{"path": "/segments/my-segment", "version": 8}`,
			}

			waitForDelete(t, store, Segments, "my-segment")
		})
	})

	t.Run("indirect flag patch", func(t *testing.T) {
		runStreamingTest(t, initialPutEvent, func(events chan<- eventsource.Event, store FeatureStore) {
			events <- &testEvent{
				event: indirectPatchEvent,
				data:  "/flags/my-flag",
			}

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
		if err != nil && item.GetVersion() == version || time.Now().After(deadline) {
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
		if err != nil && item == nil || time.Now().After(deadline) {
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
	defer ts.Close()

	cfg := Config{
		StreamUri:    ts.URL,
		FeatureStore: NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0)),
		Logger:       log.New(ioutil.Discard, "", 0),
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
}

func testStreamProcessorRecoverableError(t *testing.T, statusCode int) {
	initialPutEvent := &testEvent{
		event: putEvent,
		data: `{"path": "/", "data": {
"flags": {"my-flag": {"key": "my-flag", "version": 2}}, 
"segments": {"my-segment": {"key": "my-segment", "version": 5}}
}}`,
	}
	esserver := eventsource.NewServer()
	esserver.ReplayAll = true
	esserver.Register("test", &testRepo{initialEvent: initialPutEvent})

	attempt := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempt == 0 {
			w.WriteHeader(statusCode)
		} else {
			esserver.Handler("test").ServeHTTP(w, r)
		}
		attempt++
	}))
	defer ts.Close()

	cfg := Config{
		StreamUri:    ts.URL,
		FeatureStore: NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0)),
		Logger:       log.New(ioutil.Discard, "", 0),
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
}

func TestStreamProcessorUsesHTTPAdapter(t *testing.T) {
	polledURLs := make(chan string, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polledURLs <- r.URL.Path
		// Don't return a response because we don't want the stream to close and reconnect
	}))
	defer ts.Close()
	defer ts.CloseClientConnections()

	store := NewInMemoryFeatureStore(nil)

	cfg := Config{
		FeatureStore: store,
		Logger:       log.New(ioutil.Discard, "", 0),
		StreamUri:    ts.URL,
		HTTPAdapter:  urlAppendingHTTPAdapter("/transformed"),
	}

	sp := newStreamProcessor("sdkKey", cfg, nil)
	defer sp.Close()
	closeWhenReady := make(chan struct{})
	sp.Start(closeWhenReady)

	polledURL := <-polledURLs

	assert.Equal(t, "/all/transformed", polledURL)
}

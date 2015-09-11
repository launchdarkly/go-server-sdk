package ldclient

import (
	"encoding/json"
	"errors"
	es "github.com/launchdarkly/eventsource"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	PUT    = "put"
	PATCH  = "patch"
	DELETE = "delete"
)

type StreamProcessor struct {
	store        FeatureStore
	stream       *es.Stream
	config       Config
	disconnected *time.Time
	apiKey       string
	sync.RWMutex
}

type FeatureStore interface {
	Get(key string) (*Feature, error)
	All() (map[string]*Feature, error)
	Init(map[string]*Feature) error
	Delete(key string, version int) error
	Upsert(key string, f Feature) error
	Initialized() bool
}

type FeaturePatchData struct {
	Path string  `json:"path"`
	Data Feature `json:"data"`
}

type FeatureDeleteData struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
}

func (sp *StreamProcessor) Initialized() bool {
	return sp.store.Initialized()
}

func (sp *StreamProcessor) GetFeature(key string) (*Feature, error) {
	if !sp.store.Initialized() {
		return nil, errors.New("Requested stream data before initialization completed")
	} else {
		return sp.store.Get(key)
	}
}

func newStream(apiKey string, config Config) *StreamProcessor {
	var store FeatureStore

	if config.FeatureStore != nil {
		store = config.FeatureStore
	} else {
		store = NewInMemoryFeatureStore()
	}

	sp := &StreamProcessor{
		store:  store,
		config: config,
		apiKey: apiKey,
	}

	if !config.UseLdd {
		go sp.Start()

		go sp.Errors()
	}

	return sp
}

func (sp *StreamProcessor) subscribe() {
	sp.Lock()
	defer sp.Unlock()

	if sp.stream == nil {
		headers := make(http.Header)

		headers.Add("Authorization", "api_key "+sp.apiKey)
		headers.Add("User-Agent", "GoClient/"+Version)

		if stream, err := es.Subscribe(sp.config.StreamUri+"/features", headers, ""); err != nil {
			sp.config.Logger.Printf("Error subscribing to stream: %+v", err)
		} else {
			sp.stream = stream
		}
	}
}

func (sp *StreamProcessor) checkSubscribe() bool {
	sp.RLock()
	if sp.stream == nil {
		sp.RUnlock()
		sp.subscribe()
		return sp.stream != nil
	} else {
		defer sp.RUnlock()
		return true
	}
}

func (sp *StreamProcessor) Errors() {
	for {
		subscribed := sp.checkSubscribe()
		if !subscribed {
			sp.setDisconnected()
			time.Sleep(2 * time.Second)
			continue
		}
		err := <-sp.stream.Errors

		if err != io.EOF {
			sp.config.Logger.Printf("Error encountered processing stream: %+v", err)
		}
		if err != nil {
			sp.setDisconnected()
		}
	}
}

func (sp *StreamProcessor) setConnected() {
	sp.RLock()
	if sp.disconnected != nil {
		sp.RUnlock()
		sp.Lock()
		defer sp.Unlock()
		if sp.disconnected != nil {
			sp.disconnected = nil
		}
	} else {
		sp.RUnlock()
	}

}

func (sp *StreamProcessor) setDisconnected() {
	sp.RLock()
	if sp.disconnected == nil {
		sp.RUnlock()
		sp.Lock()
		defer sp.Unlock()
		if sp.disconnected == nil {
			now := time.Now()
			sp.disconnected = &now
		}
	} else {
		sp.RUnlock()
	}
}

func (sp *StreamProcessor) ShouldFallbackUpdate() bool {
	sp.RLock()
	defer sp.RUnlock()
	return sp.disconnected != nil && sp.disconnected.Before(time.Now().Add(-2*time.Minute))
}

func (sp *StreamProcessor) Start() {
	for {
		subscribed := sp.checkSubscribe()
		if !subscribed {
			sp.setDisconnected()
			time.Sleep(2 * time.Second)
			continue
		}
		event := <-sp.stream.Events
		switch event.Event() {
		case PUT:
			var features map[string]*Feature
			if err := json.Unmarshal([]byte(event.Data()), &features); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature json: %+v", err)
			} else {
				sp.store.Init(features)
				sp.setConnected()
			}
		case PATCH:
			var patch FeaturePatchData
			if err := json.Unmarshal([]byte(event.Data()), &patch); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature patch json: %+v", err)
			} else {
				key := strings.TrimLeft(patch.Path, "/")
				sp.store.Upsert(key, patch.Data)
				sp.setConnected()
			}
		case DELETE:
			var data FeatureDeleteData
			if err := json.Unmarshal([]byte(event.Data()), &data); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature delete json: %+v", err)
			} else {
				key := strings.TrimLeft(data.Path, "/")
				sp.store.Delete(key, data.Version)
				sp.setConnected()
			}
		default:
			sp.config.Logger.Printf("Unexpected event found in stream: %s", event.Event())
			sp.setConnected()
		}
	}
}

// A memory based FeatureStore implementation
type InMemoryFeatureStore struct {
	features      map[string]*Feature
	isInitialized bool
	sync.RWMutex
}

func NewInMemoryFeatureStore() *InMemoryFeatureStore {
	return &InMemoryFeatureStore{
		features:      make(map[string]*Feature),
		isInitialized: false,
	}
}

func (store *InMemoryFeatureStore) Get(key string) (*Feature, error) {
	store.RLock()
	defer store.RUnlock()
	f := store.features[key]

	if f == nil || f.Deleted {
		return nil, nil
	} else {
		return f, nil
	}
}

func (store *InMemoryFeatureStore) All() (map[string]*Feature, error) {
	store.RLock()
	defer store.RUnlock()
	fs := make(map[string]*Feature)

	for k, v := range store.features {
		if !v.Deleted {
			fs[k] = v
		}
	}
	return fs, nil
}

func (store *InMemoryFeatureStore) Delete(key string, version int) error {
	store.Lock()
	defer store.Unlock()
	f := store.features[key]
	if f != nil && f.Version < version {
		f.Deleted = true
		f.Version = version
		store.features[key] = f
	} else if f == nil {
		f = &Feature{Deleted: true, Version: version}
		store.features[key] = f
	}
	return nil
}

func (store *InMemoryFeatureStore) Init(fs map[string]*Feature) error {
	store.Lock()
	defer store.Unlock()

	store.features = make(map[string]*Feature)

	for k, v := range fs {
		store.features[k] = v
	}
	store.isInitialized = true
	return nil
}

func (store *InMemoryFeatureStore) Upsert(key string, f Feature) error {
	store.Lock()
	defer store.Unlock()
	old := store.features[key]

	if old == nil || old.Version < f.Version {
		store.features[key] = &f
	}
	return nil
}

func (store *InMemoryFeatureStore) Initialized() bool {
	store.RLock()
	defer store.RUnlock()
	return store.isInitialized
}

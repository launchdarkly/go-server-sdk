package ldclient

import (
	"encoding/json"
	"errors"
	es "github.com/launchdarkly/eventsource"
	"net/http"
	"strings"
	"sync"
)

var (
	PUT_FEATURE    = "put/features"
	PATCH_FEATURE  = "patch/features"
	DELETE_FEATURE = "delete/features"
)

type StreamProcessor struct {
	store  FeatureStore
	stream *es.Stream
	config Config
}

type FeatureStore interface {
	Get(key string) (*Feature, error)
	All() (map[string]*Feature, error)
	Init(map[string]*Feature) error
	Delete(key string) error
	Upsert(key string, f Feature) error
	Initialized() bool
}

type FeaturePatchData struct {
	Path string  `json:"path"`
	Data Feature `json:"data"`
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

func NewStream(apiKey string, config Config) (*StreamProcessor, error) {
	store := NewInMemoryFeatureStore()

	return NewStreamWithStore(apiKey, config, store)
}

func NewStreamWithStore(apiKey string, config Config, store FeatureStore) (*StreamProcessor, error) {
	headers := make(http.Header)

	headers.Add("Authorization", "api_key "+apiKey)
	headers.Add("User-Agent", "GoClient/"+Version)

	stream, err := es.Subscribe(config.StreamUri, headers, "")

	if err != nil {
		return nil, err
	}

	sp := &StreamProcessor{
		store:  store,
		stream: stream,
		config: config,
	}

	go sp.Start()

	return sp, nil

}

func (sp *StreamProcessor) Start() {
	for {
		event := <-sp.stream.Events
		switch event.Event() {
		case PUT_FEATURE:
			var features map[string]*Feature
			if err := json.Unmarshal([]byte(event.Data()), &features); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature json: %+v", err)
			} else {
				sp.store.Init(features)
			}
		case PATCH_FEATURE:
			var patch FeaturePatchData
			if err := json.Unmarshal([]byte(event.Data()), &patch); err != nil {
				sp.config.Logger.Printf("Unexpected error unmarshalling feature patch json: %+v", err)
			} else {
				key := strings.TrimLeft(patch.Path, "/")
				sp.store.Upsert(key, patch.Data)
			}
		default:
			sp.config.Logger.Printf("Unexpected event found in stream: %s", event.Event())
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
	return store.features[key], nil
}

func (store *InMemoryFeatureStore) All() (map[string]*Feature, error) {
	store.RLock()
	defer store.RUnlock()
	fs := make(map[string]*Feature)

	for k, v := range store.features {
		fs[k] = v
	}
	return fs, nil
}

func (store *InMemoryFeatureStore) Delete(key string) error {
	store.Lock()
	defer store.Unlock()
	delete(store.features, key)
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

	store.features[key] = &f
	return nil
}

func (store *InMemoryFeatureStore) Initialized() bool {
	store.RLock()
	defer store.RUnlock()
	return store.isInitialized
}

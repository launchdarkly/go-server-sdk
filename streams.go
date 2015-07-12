package ldclient

import (
	es "github.com/donovanhide/eventsource"
	"sync"
)

type StreamProcessor struct {
	store  StreamStore
	stream *es.Stream
}

type StreamStore interface {
	Get(key string) *Feature
	All() map[string]*Feature
	Set(map[string]*Feature)
	Delete(key string)
	Upsert(key string, f *Feature)
}

func NewStream(url string) (*StreamProcessor, error) {
	stream, err := es.Subscribe(url, "")

	return &StreamProcessor{
		store:  NewInMemoryFeatureStore(),
		stream: stream,
	}, err
}

// A memory based StreamStore implementation
type InMemoryFeatureStore struct {
	Features map[string]*Feature
	sync.RWMutex
}

func NewInMemoryFeatureStore() *InMemoryFeatureStore {
	return &InMemoryFeatureStore{
		Features: make(map[string]*Feature),
	}
}

func (store *InMemoryFeatureStore) Get(key string) *Feature {
	store.RLock()
	defer store.RUnlock()
	return store.Features[key]
}

func (store *InMemoryFeatureStore) All() map[string]*Feature {
	store.RLock()
	defer store.RUnlock()
	fs := make(map[string]*Feature)

	for k, v := range store.Features {
		fs[k] = v
	}
	return fs
}

func (store *InMemoryFeatureStore) Delete(key string) {
	store.Lock()
	defer store.Unlock()
	delete(store.Features, key)
}

func (store *InMemoryFeatureStore) Set(fs map[string]*Feature) {
	store.Lock()
	defer store.Unlock()

	store.Features = make(map[string]*Feature)

	for k, v := range fs {
		store.Features[k] = v
	}
}

func (store *InMemoryFeatureStore) Upsert(key string, f *Feature) {
	store.Lock()
	defer store.Unlock()

	store.Features[key] = f
}

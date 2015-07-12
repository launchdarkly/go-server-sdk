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
		store:  NewMemoryStreamStore(),
		stream: stream,
	}, err
}

// A memory based StreamStore implementation
type MemoryStreamStore struct {
	Features map[string]*Feature
	sync.RWMutex
}

func NewMemoryStreamStore() *MemoryStreamStore {
	return &MemoryStreamStore{
		Features: make(map[string]*Feature),
	}
}

func (store *MemoryStreamStore) Get(key string) *Feature {
	store.RLock()
	defer store.RUnlock()
	return store.Features[key]
}

func (store *MemoryStreamStore) All() map[string]*Feature {
	store.RLock()
	defer store.RUnlock()
	fs := make(map[string]*Feature)

	for k, v := range store.Features {
		fs[k] = v
	}
	return fs
}

func (store *MemoryStreamStore) Delete(key string) {
	store.Lock()
	defer store.Unlock()
	delete(store.Features, key)
}

func (store *MemoryStreamStore) Set(fs map[string]*Feature) {
	store.Lock()
	defer store.Unlock()

	store.Features = make(map[string]*Feature)

	for k, v := range fs {
		store.Features[k] = v
	}
}

func (store *MemoryStreamStore) Upsert(key string, f *Feature) {
	store.Lock()
	defer store.Unlock()

	store.Features[key] = f
}

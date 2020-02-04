package ldclient

import (
	"sync"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// DataStoreFactory is a factory function that produces a DataStore implementation. It receives
// a copy of the Config so that it can use the same logging configuration as the rest of the SDK; it
// can assume that config.Loggers has been initialized so it can write to any log level.
type DataStoreFactory func(config Config) (interfaces.DataStore, error)

// InMemoryDataStore is a memory based DataStore implementation, backed by a lock-striped map.
type InMemoryDataStore struct {
	allData       map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData
	isInitialized bool
	sync.RWMutex
	loggers ldlog.Loggers
}

// NewInMemoryDataStoreFactory returns a factory function to create an in-memory DataStore.
// Setting the DataStoreFactory option in Config to this function ensures that it will use the
// same logging configuration as the other SDK components.
func NewInMemoryDataStoreFactory() DataStoreFactory {
	return func(config Config) (interfaces.DataStore, error) {
		return newInMemoryDataStoreInternal(config), nil
	}
}

func newInMemoryDataStoreInternal(config Config) *InMemoryDataStore {
	loggers := config.Loggers
	loggers.SetPrefix("InMemoryDataStore:")
	return &InMemoryDataStore{
		allData:       make(map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData),
		isInitialized: false,
		loggers:       loggers,
	}
}

// Get returns an individual object of a given type from the store
func (store *InMemoryDataStore) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	store.RLock()
	defer store.RUnlock()
	if store.allData[kind] == nil {
		store.allData[kind] = make(map[string]interfaces.VersionedData)
	}
	item := store.allData[kind][key]

	if item == nil {
		store.loggers.Debugf(`Key %s not found in "%s"`, key, kind)
		return nil, nil
	} else if item.IsDeleted() {
		store.loggers.Debugf(`Attempted to get deleted item with key %s in "%s"`, kind, key)
		return nil, nil
	} else {
		return item, nil
	}
}

// All returns all the objects of a given kind from the store
func (store *InMemoryDataStore) All(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	store.RLock()
	defer store.RUnlock()
	ret := make(map[string]interfaces.VersionedData)

	for k, v := range store.allData[kind] {
		if !v.IsDeleted() {
			ret[k] = v
		}
	}
	return ret, nil
}

// Delete removes an item of a given kind from the store
func (store *InMemoryDataStore) Delete(kind interfaces.VersionedDataKind, key string, version int) error {
	store.Lock()
	defer store.Unlock()
	if store.allData[kind] == nil {
		store.allData[kind] = make(map[string]interfaces.VersionedData)
	}
	items := store.allData[kind]
	item := items[key]
	if item == nil || item.GetVersion() < version {
		deletedItem := kind.MakeDeletedItem(key, version)
		items[key] = deletedItem
	}
	return nil
}

// Init populates the store with a complete set of versioned data
func (store *InMemoryDataStore) Init(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	store.Lock()
	defer store.Unlock()

	store.allData = make(map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData)

	for k, v := range allData {
		items := make(map[string]interfaces.VersionedData)
		for k1, v1 := range v {
			items[k1] = v1
		}
		store.allData[k] = items
	}

	store.isInitialized = true
	return nil
}

// Upsert inserts or replaces an item in the store unless there it already contains an item with an equal or larger version
func (store *InMemoryDataStore) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) error {
	store.Lock()
	defer store.Unlock()
	if store.allData[kind] == nil {
		store.allData[kind] = make(map[string]interfaces.VersionedData)
	}
	items := store.allData[kind]
	old := items[item.GetKey()]

	if old == nil || old.GetVersion() < item.GetVersion() {
		items[item.GetKey()] = item
	}
	return nil
}

// Initialized returns whether the store has been initialized with data
func (store *InMemoryDataStore) Initialized() bool {
	store.RLock()
	defer store.RUnlock()
	return store.isInitialized
}

// Used internally to describe this component in diagnostic data.
func (store *InMemoryDataStore) GetDiagnosticsComponentTypeName() string {
	return "memory"
}

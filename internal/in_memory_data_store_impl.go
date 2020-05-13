package internal

import (
	"sync"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// inMemoryDataStore is a memory based DataStore implementation, backed by a lock-striped map.
type inMemoryDataStore struct {
	allData       map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData
	isInitialized bool
	sync.RWMutex
	loggers ldlog.Loggers
}

type inMemoryDataStoreFactory struct{}

func NewInMemoryDataStore(loggers ldlog.Loggers) interfaces.DataStore {
	return &inMemoryDataStore{
		allData:       make(map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData),
		isInitialized: false,
		loggers:       loggers,
	}
}

// Get returns an individual object of a given type from the store
func (store *inMemoryDataStore) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
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

func (store *inMemoryDataStore) All(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
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

func (store *inMemoryDataStore) Delete(kind interfaces.VersionedDataKind, key string, version int) error {
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

func (store *inMemoryDataStore) Init(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
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

func (store *inMemoryDataStore) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) error {
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

func (store *inMemoryDataStore) Initialized() bool {
	store.RLock()
	defer store.RUnlock()
	return store.isInitialized
}

func (store *inMemoryDataStore) Close() error {
	return nil
}

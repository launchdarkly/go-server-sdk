package internal

import (
	"sync"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// inMemoryDataStore is a memory based DataStore implementation, backed by a lock-striped map.
type inMemoryDataStore struct {
	allData       map[interfaces.StoreDataKind]map[string]interfaces.StoreItemDescriptor
	isInitialized bool
	sync.RWMutex
	loggers ldlog.Loggers
}

// NewInMemoryDataStore creates an instance of the in-memory data store. This is not part of the public API; it is
// always called through ldcomponents.inMemoryDataStore().
func NewInMemoryDataStore(loggers ldlog.Loggers) interfaces.DataStore {
	return &inMemoryDataStore{
		allData:       make(map[interfaces.StoreDataKind]map[string]interfaces.StoreItemDescriptor),
		isInitialized: false,
		loggers:       loggers,
	}
}

func (store *inMemoryDataStore) Init(allData []interfaces.StoreCollection) error {
	store.Lock()
	defer store.Unlock()

	store.allData = make(map[interfaces.StoreDataKind]map[string]interfaces.StoreItemDescriptor)

	for _, coll := range allData {
		items := make(map[string]interfaces.StoreItemDescriptor)
		for _, item := range coll.Items {
			items[item.Key] = item.Item
		}
		store.allData[coll.Kind] = items
	}

	store.isInitialized = true
	return nil
}

func (store *inMemoryDataStore) Get(kind interfaces.StoreDataKind, key string) (interfaces.StoreItemDescriptor, error) {
	store.RLock()
	defer store.RUnlock()
	if coll, ok := store.allData[kind]; ok {
		if item, ok := coll[key]; ok {
			return item, nil
		}
	}
	store.loggers.Debugf(`Key %s not found in "%s"`, key, kind)
	return interfaces.StoreItemDescriptor{}.NotFound(), nil
}

func (store *inMemoryDataStore) GetAll(kind interfaces.StoreDataKind) ([]interfaces.StoreKeyedItemDescriptor, error) {
	store.RLock()
	defer store.RUnlock()
	if coll, ok := store.allData[kind]; ok {
		ret := make([]interfaces.StoreKeyedItemDescriptor, 0, len(coll))
		for key, item := range coll {
			ret = append(ret, interfaces.StoreKeyedItemDescriptor{Key: key, Item: item})
		}
		return ret, nil
	}
	return nil, nil
}

func (store *inMemoryDataStore) Upsert(kind interfaces.StoreDataKind, key string, newItem interfaces.StoreItemDescriptor) error {
	store.Lock()
	defer store.Unlock()
	if coll, ok := store.allData[kind]; ok {
		if item, ok := coll[key]; ok {
			if item.Version >= newItem.Version {
				return nil
			}
		}
		coll[key] = newItem
	} else {
		store.allData[kind] = map[string]interfaces.StoreItemDescriptor{key: newItem}
	}
	return nil
}

func (store *inMemoryDataStore) IsInitialized() bool {
	store.RLock()
	defer store.RUnlock()
	return store.isInitialized
}

func (store *inMemoryDataStore) IsStatusMonitoringEnabled() bool {
	return false
}

func (store *inMemoryDataStore) Close() error {
	return nil
}

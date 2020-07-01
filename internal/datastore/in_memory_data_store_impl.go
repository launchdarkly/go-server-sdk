package datastore

import (
	"sync"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
)

// inMemoryDataStore is a memory based DataStore implementation, backed by a lock-striped map.
//
// Implementation notes:
//
// We deliberately do not use a defer pattern to manage the lock in these methods. Using defer adds a small but
// consistent overhead, and these store methods may be called with very high frequency (at least in the case of
// Get and IsInitialized). To make it safe to hold a lock without deferring the unlock, we must ensure that
// there is only one return point from each method, and that there is no operation that could possibly cause a
// panic after the lock has been acquired. See notes on performance in CONTRIBUTING.md.
type inMemoryDataStore struct {
	allData       map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor
	isInitialized bool
	sync.RWMutex
	loggers ldlog.Loggers
}

// NewInMemoryDataStore creates an instance of the in-memory data store. This is not part of the public API; it is
// always called through ldcomponents.inMemoryDataStore().
func NewInMemoryDataStore(loggers ldlog.Loggers) interfaces.DataStore {
	return &inMemoryDataStore{
		allData:       make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor),
		isInitialized: false,
		loggers:       loggers,
	}
}

func (store *inMemoryDataStore) Init(allData []ldstoretypes.Collection) error {
	store.Lock()

	store.allData = make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor)

	for _, coll := range allData {
		items := make(map[string]ldstoretypes.ItemDescriptor)
		for _, item := range coll.Items {
			items[item.Key] = item.Item
		}
		store.allData[coll.Kind] = items
	}

	store.isInitialized = true

	store.Unlock()

	return nil
}

func (store *inMemoryDataStore) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	store.RLock()

	var coll map[string]ldstoretypes.ItemDescriptor
	var item ldstoretypes.ItemDescriptor
	var ok bool
	coll, ok = store.allData[kind]
	if ok {
		item, ok = coll[key]
	}

	store.RUnlock()

	if ok {
		return item, nil
	}
	if store.loggers.IsDebugEnabled() {
		store.loggers.Debugf(`Key %s not found in "%s"`, key, kind)
	}
	return ldstoretypes.ItemDescriptor{}.NotFound(), nil
}

func (store *inMemoryDataStore) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	store.RLock()

	var itemsOut []ldstoretypes.KeyedItemDescriptor
	if itemsMap, ok := store.allData[kind]; ok {
		if len(itemsMap) > 0 {
			itemsOut = make([]ldstoretypes.KeyedItemDescriptor, 0, len(itemsMap))
			for key, item := range itemsMap {
				itemsOut = append(itemsOut, ldstoretypes.KeyedItemDescriptor{Key: key, Item: item})
			}
		}
	}

	store.RUnlock()

	return itemsOut, nil
}

func (store *inMemoryDataStore) Upsert(
	kind ldstoretypes.DataKind,
	key string,
	newItem ldstoretypes.ItemDescriptor,
) (bool, error) {
	store.Lock()

	var coll map[string]ldstoretypes.ItemDescriptor
	var ok bool
	shouldUpdate := true
	updated := false
	if coll, ok = store.allData[kind]; ok {
		if item, ok := coll[key]; ok {
			if item.Version >= newItem.Version {
				shouldUpdate = false
			}
		}
	} else {
		store.allData[kind] = map[string]ldstoretypes.ItemDescriptor{key: newItem}
		shouldUpdate = false // because we already initialized the map with the new item
		updated = true
	}
	if shouldUpdate {
		coll[key] = newItem
		updated = true
	}

	store.Unlock()

	return updated, nil
}

func (store *inMemoryDataStore) IsInitialized() bool {
	store.RLock()
	ret := store.isInitialized
	store.RUnlock()
	return ret
}

func (store *inMemoryDataStore) IsStatusMonitoringEnabled() bool {
	return false
}

func (store *inMemoryDataStore) Close() error {
	return nil
}

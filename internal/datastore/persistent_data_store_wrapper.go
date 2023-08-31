package datastore

import (
	"fmt"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/patrickmn/go-cache"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/singleflight"
)

// persistentDataStoreWrapper is the implementation of DataStore that we use for all persistent data stores.
type persistentDataStoreWrapper struct {
	core             subsystems.PersistentDataStore
	dataStoreUpdates subsystems.DataStoreUpdateSink
	statusPoller     *dataStoreStatusPoller
	cache            *cache.Cache
	cacheTTL         time.Duration
	requests         singleflight.Group
	loggers          ldlog.Loggers
	inited           bool
	initLock         sync.RWMutex
}

const initCheckedKey = "$initChecked"

// NewPersistentDataStoreWrapper creates the implementation of DataStore that we use for all persistent data
// stores. This is not visible in the public API; it is always called through ldcomponents.PersistentDataStore().
func NewPersistentDataStoreWrapper(
	core subsystems.PersistentDataStore,
	dataStoreUpdates subsystems.DataStoreUpdateSink,
	cacheTTL time.Duration,
	loggers ldlog.Loggers,
) subsystems.DataStore {
	var myCache *cache.Cache
	if cacheTTL != 0 {
		myCache = cache.New(cacheTTL, 5*time.Minute)
		// Note that the documented behavior of go-cache is that if cacheTTL is negative, the
		// cache never expires. That is consistent with we've defined the parameter.
	}

	w := &persistentDataStoreWrapper{
		core:             core,
		dataStoreUpdates: dataStoreUpdates,
		cache:            myCache,
		cacheTTL:         cacheTTL,
		loggers:          loggers,
	}

	w.statusPoller = newDataStoreStatusPoller(
		true,
		w.pollAvailabilityAfterOutage,
		dataStoreUpdates.UpdateStatus,
		myCache == nil || cacheTTL > 0, // needsRefresh=true unless we're in infinite cache mode
		loggers,
	)

	return w
}

func (w *persistentDataStoreWrapper) Init(allData []st.Collection) error {
	err := w.initCore(allData)
	if w.cache != nil {
		w.cache.Flush()
	}
	if err != nil && !w.hasInfiniteCache() {
		// If the underlying store failed to do the update, and we've got an expiring cache, then:
		// 1) We shouldn't update the cache, and
		// 2) We shouldn't be considered initialized.
		// The rationale is that it's better to stay in a consistent state of having old data than to act
		// like we have new data, but then suddenly fall back to old data when the cache expires.
		return err
	}
	// However, if the cache TTL is infinite, then it makes sense to update the cache regardless of the
	// initialization result of the underlying store.
	if w.cache != nil {
		for _, coll := range allData {
			w.cacheItems(coll.Kind, coll.Items)
		}
	}
	w.initLock.Lock()
	defer w.initLock.Unlock()
	w.inited = true
	return err
}

func (w *persistentDataStoreWrapper) Get(kind st.DataKind, key string) (st.ItemDescriptor, error) {
	if w.cache == nil {
		item, err := w.getAndDeserializeItem(kind, key)
		w.processError(err)
		return item, err
	}
	cacheKey := dataStoreCacheKey(kind, key)
	if data, present := w.cache.Get(cacheKey); present {
		if item, ok := data.(st.ItemDescriptor); ok {
			return item, nil
		}
	}
	// Item was not cached or cached value was not valid. Use singleflight to ensure that we'll only
	// do this core query once even if multiple goroutines are requesting it
	reqKey := fmt.Sprintf("get:%s:%s", kind.GetName(), key)
	itemIntf, err, _ := w.requests.Do(reqKey, func() (interface{}, error) {
		item, err := w.getAndDeserializeItem(kind, key)
		w.processError(err)
		if err == nil {
			w.cache.Set(cacheKey, item, cache.DefaultExpiration)
			return item, nil
		}
		return nil, err
	})
	if err != nil || itemIntf == nil {
		return st.ItemDescriptor{}.NotFound(), err
	}
	if item, ok := itemIntf.(st.ItemDescriptor); ok { // singleflight.Group.Do returns value as interface{}
		return item, err
	}
	w.loggers.Errorf("data store query returned unexpected type %T", itemIntf)
	// COVERAGE: there is no way to simulate this condition in unit tests; it should be impossible
	return st.ItemDescriptor{}.NotFound(), nil
}

func (w *persistentDataStoreWrapper) GetAll(kind st.DataKind) ([]st.KeyedItemDescriptor, error) {
	if w.cache == nil {
		items, err := w.getAllAndDeserialize(kind)
		w.processError(err)
		return items, err
	}
	// Check whether we have a cache item for the entire data set
	cacheKey := dataStoreAllItemsCacheKey(kind)
	if data, present := w.cache.Get(cacheKey); present {
		if items, ok := data.([]st.KeyedItemDescriptor); ok {
			return items, nil
		}
	}
	// Data set was not cached or cached value was not valid. Use singleflight to ensure that we'll only
	// do this core query once even if multiple goroutines are requesting it
	reqKey := fmt.Sprintf("all:%s", kind.GetName())
	itemsIntf, err, _ := w.requests.Do(reqKey, func() (interface{}, error) {
		items, err := w.getAllAndDeserialize(kind)
		w.processError(err)
		if err == nil {
			w.cache.Set(cacheKey, items, cache.DefaultExpiration)
			return items, nil
		}
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	if items, ok := itemsIntf.([]st.KeyedItemDescriptor); ok { // singleflight.Group.Do returns value as interface{}
		return items, err
	}
	w.loggers.Errorf("data store query returned unexpected type %T", itemsIntf)
	// COVERAGE: there is no way to simulate this condition in unit tests; it should be impossible
	return nil, nil
}

func (w *persistentDataStoreWrapper) Upsert(
	kind st.DataKind,
	key string,
	newItem st.ItemDescriptor,
) (bool, error) {
	serializedItem := w.serialize(kind, newItem)
	updated, err := w.core.Upsert(kind, key, serializedItem)
	w.processError(err)
	// Normally, if the underlying store failed to do the update, we do not want to update the cache -
	// the idea being that it's better to stay in a consistent state of having old data than to act
	// like we have new data but then suddenly fall back to old data when the cache expires. However,
	// if the cache TTL is infinite, then it makes sense to update the cache always.
	if err != nil {
		if !w.hasInfiniteCache() {
			return updated, err
		}
	}
	if w.cache != nil {
		cacheKey := dataStoreCacheKey(kind, key)
		allCacheKey := dataStoreAllItemsCacheKey(kind)
		if err == nil {
			if updated {
				w.cache.Set(cacheKey, newItem, cache.DefaultExpiration)
				// If the cache has a finite TTL, then we should remove the "all items" cache entry to force
				// a reread the next time All is called. However, if it's an infinite TTL, we need to just
				// update the item within the existing "all items" entry (since we want things to still work
				// even if the underlying store is unavailable).
				if w.hasInfiniteCache() {
					if data, present := w.cache.Get(allCacheKey); present {
						if items, ok := data.([]st.KeyedItemDescriptor); ok {
							w.cache.Set(allCacheKey, updateSingleItem(items, key, newItem), cache.DefaultExpiration)
						}
					}
				} else {
					w.cache.Delete(allCacheKey)
				}
			} else {
				// there was a concurrent modification elsewhere - update the cache to get the new state
				w.cache.Delete(cacheKey)
				w.cache.Delete(allCacheKey)
				_, _ = w.Get(kind, key) // doing this query repopulates the cache
			}
		} else {
			// The underlying store returned an error. If the cache has an infinite TTL, then we should go
			// ahead and update the cache so that it always has the latest data; we may be able to use the
			// cached data to repopulate the store later if it starts working again.
			if w.hasInfiniteCache() {
				w.cache.Set(cacheKey, newItem, cache.DefaultExpiration)
				cachedItems := []st.KeyedItemDescriptor{}
				if data, present := w.cache.Get(allCacheKey); present {
					if items, ok := data.([]st.KeyedItemDescriptor); ok {
						cachedItems = items
					}
				}
				w.cache.Set(allCacheKey, updateSingleItem(cachedItems, key, newItem), cache.DefaultExpiration)
			}
		}
	}
	return updated, err
}

func (w *persistentDataStoreWrapper) IsInitialized() bool {
	w.initLock.RLock()
	previousValue := w.inited
	w.initLock.RUnlock()
	if previousValue {
		return true
	}

	if w.cache != nil {
		if _, found := w.cache.Get(initCheckedKey); found {
			return false
		}
	}

	newValue := w.core.IsInitialized()
	if newValue {
		w.initLock.Lock()
		defer w.initLock.Unlock()
		w.inited = true
		if w.cache != nil {
			w.cache.Delete(initCheckedKey)
		}
	} else if w.cache != nil {
		w.cache.Set(initCheckedKey, "", cache.DefaultExpiration)
	}
	return newValue
}

func (w *persistentDataStoreWrapper) IsStatusMonitoringEnabled() bool {
	return true
}

func (w *persistentDataStoreWrapper) Close() error {
	w.statusPoller.Close()
	return w.core.Close()
}

func (w *persistentDataStoreWrapper) pollAvailabilityAfterOutage() bool {
	if !w.core.IsStoreAvailable() {
		return false
	}
	if w.hasInfiniteCache() {
		// If we're in infinite cache mode, then we can assume the cache has a full set of current
		// flag data (since presumably the data source has still been running) and we can just
		// write the contents of the cache to the underlying data store.
		kinds := datakinds.AllDataKinds()
		allData := make([]st.Collection, 0, len(kinds))
		for _, kind := range kinds {
			allCacheKey := dataStoreAllItemsCacheKey(kind)
			if data, present := w.cache.Get(allCacheKey); present {
				if items, ok := data.([]st.KeyedItemDescriptor); ok {
					allData = append(allData, st.Collection{Kind: kind, Items: items})
				}
			}
		}
		err := w.initCore(allData)
		if err != nil {
			// We failed to write the cached data to the underlying store. In this case,
			// w.initCore() has already put us back into the failed state. The only further
			// thing we can do is to log a note about what just happened.
			w.loggers.Errorf("Tried to write cached data to persistent store after a store outage, but failed: %s", err)
		} else {
			w.loggers.Warn("Successfully updated persistent store from cached data")
			// Note that w.inited should have already been set when InitInternal was originally called -
			// in infinite cache mode, we set it even if the database update failed.
		}
	}
	return true
}

func (w *persistentDataStoreWrapper) hasInfiniteCache() bool {
	return w.cache != nil && w.cacheTTL < 0
}
func dataStoreCacheKey(kind st.DataKind, key string) string {
	return kind.GetName() + ":" + key
}

func dataStoreAllItemsCacheKey(kind st.DataKind) string {
	return "all:" + kind.GetName()
}

func (w *persistentDataStoreWrapper) initCore(allData []st.Collection) error {
	serializedAllData := make([]st.SerializedCollection, 0, len(allData))
	for _, coll := range allData {
		serializedAllData = append(serializedAllData, st.SerializedCollection{
			Kind:  coll.Kind,
			Items: w.serializeAll(coll.Kind, coll.Items),
		})
	}
	err := w.core.Init(serializedAllData)
	w.processError(err)
	return err
}

func (w *persistentDataStoreWrapper) getAndDeserializeItem(
	kind st.DataKind,
	key string,
) (st.ItemDescriptor, error) {
	serializedItem, err := w.core.Get(kind, key)
	if err == nil {
		return w.deserialize(kind, serializedItem)
	}
	return st.ItemDescriptor{}.NotFound(), err
}

func (w *persistentDataStoreWrapper) getAllAndDeserialize(
	kind st.DataKind,
) ([]st.KeyedItemDescriptor, error) {
	serializedItems, err := w.core.GetAll(kind)
	if err == nil {
		ret := make([]st.KeyedItemDescriptor, 0, len(serializedItems))
		for _, serializedItem := range serializedItems {
			item, err := w.deserialize(kind, serializedItem.Item)
			if err != nil {
				return nil, err
			}
			ret = append(ret, st.KeyedItemDescriptor{Key: serializedItem.Key, Item: item})
		}
		return ret, nil
	}
	return nil, err
}

func (w *persistentDataStoreWrapper) cacheItems(
	kind st.DataKind,
	items []st.KeyedItemDescriptor,
) {
	if w.cache != nil {
		copyOfItems := slices.Clone(items)
		w.cache.Set(dataStoreAllItemsCacheKey(kind), copyOfItems, cache.DefaultExpiration)

		for _, item := range items {
			w.cache.Set(dataStoreCacheKey(kind, item.Key), item.Item, cache.DefaultExpiration)
		}
	}
}

func (w *persistentDataStoreWrapper) serialize(
	kind st.DataKind,
	item st.ItemDescriptor,
) st.SerializedItemDescriptor {
	isDeleted := item.Item == nil
	return st.SerializedItemDescriptor{
		Version:        item.Version,
		Deleted:        isDeleted,
		SerializedItem: kind.Serialize(item),
	}
}

func (w *persistentDataStoreWrapper) serializeAll(
	kind st.DataKind,
	items []st.KeyedItemDescriptor,
) []st.KeyedSerializedItemDescriptor {
	ret := make([]st.KeyedSerializedItemDescriptor, 0, len(items))
	for _, item := range items {
		ret = append(ret, st.KeyedSerializedItemDescriptor{
			Key:  item.Key,
			Item: w.serialize(kind, item.Item),
		})
	}
	return ret
}

func (w *persistentDataStoreWrapper) deserialize(
	kind st.DataKind,
	serializedItemDesc st.SerializedItemDescriptor,
) (st.ItemDescriptor, error) {
	if serializedItemDesc.Deleted || serializedItemDesc.SerializedItem == nil {
		return st.ItemDescriptor{Version: serializedItemDesc.Version}, nil
	}
	deserializedItemDesc, err := kind.Deserialize(serializedItemDesc.SerializedItem)
	if err != nil {
		return st.ItemDescriptor{}.NotFound(), err
	}
	if serializedItemDesc.Version == 0 || serializedItemDesc.Version == deserializedItemDesc.Version {
		return deserializedItemDesc, nil
	}
	// If the store gave us a version number that isn't what was encoded in the object, trust it
	return st.ItemDescriptor{Version: serializedItemDesc.Version, Item: deserializedItemDesc.Item}, nil
}

func updateSingleItem(
	items []st.KeyedItemDescriptor,
	key string,
	newItem st.ItemDescriptor,
) []st.KeyedItemDescriptor {
	found := false
	ret := make([]st.KeyedItemDescriptor, 0, len(items))
	for _, item := range items {
		if item.Key == key {
			ret = append(ret, st.KeyedItemDescriptor{Key: key, Item: newItem})
			found = true
		} else {
			ret = append(ret, item)
		}
	}
	if !found {
		ret = append(ret, st.KeyedItemDescriptor{Key: key, Item: newItem})
	}
	return ret
}

func (w *persistentDataStoreWrapper) processError(err error) {
	if err == nil {
		// If we're waiting to recover after a failure, we'll let the polling routine take care
		// of signaling success. Even if we could signal success a little earlier based on the
		// success of whatever operation we just did, we'd rather avoid the overhead of acquiring
		// w.statusLock every time we do anything. So we'll just do nothing here.
		return
	}
	w.loggers.Errorf("Data store returned error: %s", err.Error())
	w.statusPoller.UpdateAvailability(false)
}

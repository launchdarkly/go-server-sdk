package internal

import (
	"fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type PersistentDataStoreWrapper struct {
	core          intf.PersistentDataStore
	statusManager *DataStoreStatusManager
	cache         *cache.Cache
	cacheTTL      time.Duration
	requests      singleflight.Group
	loggers       ldlog.Loggers
	inited        bool
	initLock      sync.RWMutex
}

const initCheckedKey = "$initChecked"

func NewPersistentDataStoreWrapper(
	core intf.PersistentDataStore,
	cacheTTL time.Duration,
	loggers ldlog.Loggers,
) *PersistentDataStoreWrapper {
	var myCache *cache.Cache
	if cacheTTL != 0 {
		myCache = cache.New(cacheTTL, 5*time.Minute)
		// Note that the documented behavior of go-cache is that if cacheTTL is negative, the
		// cache never expires. That is consistent with we've defined the parameter.
	}

	w := &PersistentDataStoreWrapper{
		core:     core,
		cache:    myCache,
		cacheTTL: cacheTTL,
		loggers:  loggers,
	}

	w.statusManager = NewDataStoreStatusManager(
		true,
		w.pollAvailabilityAfterOutage,
		myCache == nil || cacheTTL > 0, // needsRefresh=true unless we're in infinite cache mode
		loggers,
	)

	return w
}

func (w *PersistentDataStoreWrapper) Init(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	err := w.initCore(allData)
	if w.cache != nil {
		w.cache.Flush()
	}
	if err != nil && !w.hasCacheWithInfiniteTTL() {
		// Normally, if the underlying store failed to do the update, we do not want to update the cache -
		// the idea being that it's better to stay in a consistent state of having old data than to act
		// like we have new data but then suddenly fall back to old data when the cache expires. However,
		// if the cache TTL is infinite, then it makes sense to update the cache always.
		return err
	}
	if w.cache != nil {
		for kind, items := range allData {
			w.filterAndCacheItems(kind, items)
		}
	}
	if err == nil || w.hasCacheWithInfiniteTTL() {
		w.initLock.Lock()
		defer w.initLock.Unlock()
		w.inited = true
	}
	return err
}

func (w *PersistentDataStoreWrapper) initCore(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	colls := TransformUnorderedDataToOrderedData(allData)
	err := w.core.Init(colls)
	w.processError(err)
	return err
}

func (w *PersistentDataStoreWrapper) filterAndCacheItems(kind interfaces.VersionedDataKind, items map[string]interfaces.VersionedData) map[string]interfaces.VersionedData {
	// We do some filtering here so that deleted items are not included in the full cached data set
	// that's used by All. This is so that All doesn't have to do that filtering itself. However,
	// since Get does know to filter out deleted items, we will still cache those individually,
	filteredItems := make(map[string]interfaces.VersionedData, len(items))
	for key, item := range items {
		if !item.IsDeleted() {
			filteredItems[key] = item
		}
		if w.cache != nil {
			w.cache.Set(dataStoreCacheKey(kind, key), item, cache.DefaultExpiration)
		}
	}
	if w.cache != nil {
		w.cache.Set(dataStoreAllItemsCacheKey(kind), filteredItems, cache.DefaultExpiration)
	}
	return filteredItems
}

func (w *PersistentDataStoreWrapper) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	if w.cache == nil {
		item, err := w.core.Get(kind, key)
		w.processError(err)
		return itemOnlyIfNotDeleted(item), err
	}
	cacheKey := dataStoreCacheKey(kind, key)
	if data, present := w.cache.Get(cacheKey); present {
		if data == nil { // If present is true but data is nil, we have cached the absence of an item
			return nil, nil
		}
		if item, ok := data.(interfaces.VersionedData); ok {
			return itemOnlyIfNotDeleted(item), nil
		}
	}
	// Item was not cached or cached value was not valid. Use singleflight to ensure that we'll only
	// do this core query once even if multiple goroutines are requesting it
	reqKey := fmt.Sprintf("get:%s:%s", kind.GetNamespace(), key)
	itemIntf, err, _ := w.requests.Do(reqKey, func() (interface{}, error) {
		item, err := w.core.Get(kind, key)
		w.processError(err)
		if err == nil {
			w.cache.Set(cacheKey, item, cache.DefaultExpiration)
		}
		return itemOnlyIfNotDeleted(item), err
	})
	if err != nil || itemIntf == nil {
		return nil, err
	}
	if item, ok := itemIntf.(interfaces.VersionedData); ok { // singleflight.Group.Do returns value as interface{}
		return item, err
	}
	w.loggers.Errorf("data store query returned unexpected type %T", itemIntf)
	return nil, nil
}

func itemOnlyIfNotDeleted(item interfaces.VersionedData) interfaces.VersionedData {
	if item != nil && item.IsDeleted() {
		return nil
	}
	return item
}

func (w *PersistentDataStoreWrapper) All(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	if w.cache == nil {
		items, err := w.core.GetAll(kind)
		w.processError(err)
		return items, err
	}
	// Check whether we have a cache item for the entire data set
	cacheKey := dataStoreAllItemsCacheKey(kind)
	if data, present := w.cache.Get(cacheKey); present {
		if items, ok := data.(map[string]interfaces.VersionedData); ok {
			return items, nil
		}
	}
	// Data set was not cached or cached value was not valid. Use singleflight to ensure that we'll only
	// do this core query once even if multiple goroutines are requesting it
	reqKey := fmt.Sprintf("all:%s", kind.GetNamespace())
	itemsIntf, err, _ := w.requests.Do(reqKey, func() (interface{}, error) {
		items, err := w.core.GetAll(kind)
		w.processError(err)
		if err != nil {
			return nil, err
		}
		return w.filterAndCacheItems(kind, items), nil
	})
	if err != nil {
		return nil, err
	}
	if items, ok := itemsIntf.(map[string]interfaces.VersionedData); ok { // singleflight.Group.Do returns value as interface{}
		return items, err
	}
	w.loggers.Errorf("data store query returned unexpected type %T", itemsIntf)
	return nil, nil
}

func (w *PersistentDataStoreWrapper) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) error {
	finalItem, err := w.core.Upsert(kind, item)
	w.processError(err)
	// Normally, if the underlying store failed to do the update, we do not want to update the cache -
	// the idea being that it's better to stay in a consistent state of having old data than to act
	// like we have new data but then suddenly fall back to old data when the cache expires. However,
	// if the cache TTL is infinite, then it makes sense to update the cache always.
	if err != nil {
		if !w.hasCacheWithInfiniteTTL() {
			return err
		}
		finalItem = item
	}
	// Note that what we put into the cache is finalItem, which may not be the same as item (i.e. if
	// another process has already updated the item to a higher version).
	if finalItem != nil && w.cache != nil {
		w.cache.Set(dataStoreCacheKey(kind, item.GetKey()), finalItem, cache.DefaultExpiration)
		// If the cache has a finite TTL, then we should remove the "all items" cache entry to force
		// a reread the next time All is called. However, if it's an infinite TTL, we need to just
		// update the item within the existing "all items" entry (since we want things to still work
		// even if the underlying store is unavailable).
		allCacheKey := dataStoreAllItemsCacheKey(kind)
		if w.hasCacheWithInfiniteTTL() {
			if data, present := w.cache.Get(allCacheKey); present {
				if items, ok := data.(map[string]interfaces.VersionedData); ok {
					items[item.GetKey()] = item // updates the existing map since maps are passed by reference
				}
			} else {
				items := map[string]interfaces.VersionedData{item.GetKey(): item}
				w.cache.Set(allCacheKey, items, cache.DefaultExpiration)
			}
		} else {
			w.cache.Delete(allCacheKey)
		}
	}
	return err
}

func (w *PersistentDataStoreWrapper) Delete(kind interfaces.VersionedDataKind, key string, version int) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return w.Upsert(kind, deletedItem)
}

func (w *PersistentDataStoreWrapper) Initialized() bool {
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
	} else {
		if w.cache != nil {
			w.cache.Set(initCheckedKey, "", cache.DefaultExpiration)
		}
	}
	return newValue
}

func (w *PersistentDataStoreWrapper) Close() error {
	w.statusManager.Close()
	return w.core.Close()
}

// GetStoreStatus returns the current status of the store.
func (w *PersistentDataStoreWrapper) GetStoreStatus() DataStoreStatus {
	return DataStoreStatus{Available: w.statusManager.IsAvailable()}
}

// StatusSubscribe creates a channel that will receive all changes in store status.
func (w *PersistentDataStoreWrapper) StatusSubscribe() DataStoreStatusSubscription {
	return w.statusManager.Subscribe()
}

// Used internally to describe this component in diagnostic data.
func (w *PersistentDataStoreWrapper) DescribeConfiguration() ldvalue.Value {
	if dcd, ok := w.core.(intf.DiagnosticDescription); ok {
		return dcd.DescribeConfiguration()
	}
	return ldvalue.String("custom")
}

func (w *PersistentDataStoreWrapper) processError(err error) {
	if err == nil {
		// If we're waiting to recover after a failure, we'll let the polling routine take care
		// of signaling success. Even if we could signal success a little earlier based on the
		// success of whatever operation we just did, we'd rather avoid the overhead of acquiring
		// w.statusLock every time we do anything. So we'll just do nothing here.
		return
	}
	w.statusManager.UpdateAvailability(false)
}

func (w *PersistentDataStoreWrapper) pollAvailabilityAfterOutage() bool {
	if !w.core.IsStoreAvailable() {
		return false
	}
	if w.hasCacheWithInfiniteTTL() {
		// If we're in infinite cache mode, then we can assume the cache has a full set of current
		// flag data (since presumably the data source has still been running) and we can just
		// write the contents of the cache to the underlying data store.
		allData := make(map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData, 2)
		for _, kind := range interfaces.VersionedDataKinds() {
			allCacheKey := dataStoreAllItemsCacheKey(kind)
			if data, present := w.cache.Get(allCacheKey); present {
				if items, ok := data.(map[string]interfaces.VersionedData); ok {
					allData[kind] = items
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

func (w *PersistentDataStoreWrapper) hasCacheWithInfiniteTTL() bool {
	return w.cache != nil && w.cacheTTL < 0
}

func dataStoreCacheKey(kind intf.VersionedDataKind, key string) string {
	return kind.GetNamespace() + ":" + key
}

func dataStoreAllItemsCacheKey(kind intf.VersionedDataKind) string {
	return "all:" + kind.GetNamespace()
}

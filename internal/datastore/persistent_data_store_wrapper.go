package datastore

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"slices"

	"github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

// persistentDataStoreWrapper is the implementation of DataStore that we use for all persistent data stores.
//
// The cache is held behind an atomic.Pointer so that DropCache can swap it out
// for nil without blocking readers. Each method that touches the cache calls
// Load once at the top and works with that local snapshot for the rest of the
// call -- so a concurrent drop can never observe a half-modified cache, and
// readers never block on a mutex.
type persistentDataStoreWrapper struct {
	core             subsystems.PersistentDataStore
	dataStoreUpdates subsystems.DataStoreUpdateSink
	statusPoller     *dataStoreStatusPoller
	cache            atomic.Pointer[cache.Cache]
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
	w := &persistentDataStoreWrapper{
		core:             core,
		dataStoreUpdates: dataStoreUpdates,
		cacheTTL:         cacheTTL,
		loggers:          loggers,
	}

	if cacheTTL != 0 {
		// Note that the documented behavior of go-cache is that if cacheTTL is negative, the
		// cache never expires. That is consistent with how we've defined the parameter.
		w.cache.Store(cache.New(cacheTTL, 5*time.Minute))
	}

	w.statusPoller = newDataStoreStatusPoller(
		true,
		w.pollAvailabilityAfterOutage,
		dataStoreUpdates.UpdateStatus,
		cacheTTL >= 0, // needsRefresh=true unless we're in infinite cache mode (cacheTTL < 0)
		loggers,
	)

	return w
}

func (w *persistentDataStoreWrapper) Init(allData []st.Collection) error {
	err := w.initCore(allData)
	c := w.cache.Load()
	if c != nil {
		c.Flush()
	}
	if err != nil && !(c != nil && w.cacheTTL < 0) {
		// If the underlying store failed to do the update, and we've got an expiring cache, then:
		// 1) We shouldn't update the cache, and
		// 2) We shouldn't be considered initialized.
		// The rationale is that it's better to stay in a consistent state of having old data than to act
		// like we have new data, but then suddenly fall back to old data when the cache expires.
		return err
	}
	// However, if the cache TTL is infinite, then it makes sense to update the cache regardless of the
	// initialization result of the underlying store.
	if c != nil {
		for _, coll := range allData {
			cacheCollection(c, coll.Kind, coll.Items)
		}
	}
	w.initLock.Lock()
	defer w.initLock.Unlock()
	w.inited = true
	return err
}

func (w *persistentDataStoreWrapper) Get(kind st.DataKind, key string) (st.ItemDescriptor, error) {
	c := w.cache.Load()
	if c == nil {
		item, err := w.getAndDeserializeItem(kind, key)
		w.processError(err)
		return item, err
	}
	cacheKey := dataStoreCacheKey(kind, key)
	if data, present := c.Get(cacheKey); present {
		if item, ok := data.(st.ItemDescriptor); ok {
			return item, nil
		}
	}
	// Item was not cached or cached value was not valid. Use singleflight to ensure that we'll only
	// do this core query once even if multiple goroutines are requesting it.
	reqKey := fmt.Sprintf("get:%s:%s", kind.GetName(), key)
	itemIntf, err, _ := w.requests.Do(reqKey, func() (interface{}, error) {
		item, err := w.getAndDeserializeItem(kind, key)
		w.processError(err)
		if err == nil {
			// Re-load in case the cache was dropped while we were waiting on the core.
			if c := w.cache.Load(); c != nil {
				c.Set(cacheKey, item, cache.DefaultExpiration)
			}
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
	c := w.cache.Load()
	if c == nil {
		items, err := w.getAllAndDeserialize(kind)
		w.processError(err)
		return items, err
	}
	cacheKey := dataStoreAllItemsCacheKey(kind)
	if data, present := c.Get(cacheKey); present {
		if items, ok := data.([]st.KeyedItemDescriptor); ok {
			return items, nil
		}
	}
	// Data set was not cached or cached value was not valid. Use singleflight to ensure that we'll only
	// do this core query once even if multiple goroutines are requesting it.
	reqKey := fmt.Sprintf("all:%s", kind.GetName())
	itemsIntf, err, _ := w.requests.Do(reqKey, func() (interface{}, error) {
		items, err := w.getAllAndDeserialize(kind)
		w.processError(err)
		if err == nil {
			if c := w.cache.Load(); c != nil {
				c.Set(cacheKey, items, cache.DefaultExpiration)
			}
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

	c := w.cache.Load()
	infinite := w.cacheTTL < 0

	// Normally, if the underlying store failed to do the update, we do not want to update the cache:
	// it's better to stay in a consistent state of having old data than to act like we have new data
	// but then suddenly fall back to old data when the cache expires. The exception is infinite-TTL
	// mode, where we keep the cache in sync regardless so it can repopulate the store after a recovered
	// outage.
	if err != nil && !(c != nil && infinite) {
		return updated, err
	}
	if c == nil {
		return updated, err
	}

	cacheKey := dataStoreCacheKey(kind, key)
	allCacheKey := dataStoreAllItemsCacheKey(kind)

	if err == nil {
		if updated {
			c.Set(cacheKey, newItem, cache.DefaultExpiration)
			// Finite TTL: drop the "all items" entry to force a reread next time GetAll is called.
			// Infinite TTL: update the entry in place so things still work if the store is unavailable.
			if infinite {
				if data, present := c.Get(allCacheKey); present {
					if items, ok := data.([]st.KeyedItemDescriptor); ok {
						c.Set(allCacheKey, updateSingleItem(items, key, newItem), cache.DefaultExpiration)
					}
				}
			} else {
				c.Delete(allCacheKey)
			}
		} else {
			// Concurrent modification elsewhere -- drop our cached values and refetch.
			c.Delete(cacheKey)
			c.Delete(allCacheKey)
			_, _ = w.Get(kind, key) // doing this query repopulates the cache
		}
	} else {
		// err != nil and infinite cache mode (we already returned for the !infinite case).
		// Update the cache so it always has the latest data; we may be able to use it to repopulate
		// the store later if it starts working again.
		c.Set(cacheKey, newItem, cache.DefaultExpiration)
		cachedItems := []st.KeyedItemDescriptor{}
		if data, present := c.Get(allCacheKey); present {
			if items, ok := data.([]st.KeyedItemDescriptor); ok {
				cachedItems = items
			}
		}
		c.Set(allCacheKey, updateSingleItem(cachedItems, key, newItem), cache.DefaultExpiration)
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

	c := w.cache.Load()
	if c != nil {
		if _, found := c.Get(initCheckedKey); found {
			return false
		}
	}

	newValue := w.core.IsInitialized()
	if newValue {
		w.initLock.Lock()
		w.inited = true
		w.initLock.Unlock()
		if c != nil {
			c.Delete(initCheckedKey)
		}
	} else if c != nil {
		c.Set(initCheckedKey, "", cache.DefaultExpiration)
	}
	return newValue
}

func (w *persistentDataStoreWrapper) IsStatusMonitoringEnabled() bool {
	return true
}

func (w *persistentDataStoreWrapper) Close() error {
	w.DropCache()
	w.statusPoller.Close()
	return w.core.Close()
}

// DropCache flushes and releases the in-memory cache. Called once the FDv2
// in-memory store has been initialized and is the source of truth. Safe to
// call multiple times.
func (w *persistentDataStoreWrapper) DropCache() {
	if c := w.cache.Swap(nil); c != nil {
		c.Flush()
		w.loggers.Debug("Persistent store cache dropped; in-memory store is now active")
	}
}

func (w *persistentDataStoreWrapper) pollAvailabilityAfterOutage() bool {
	if !w.core.IsStoreAvailable() {
		return false
	}
	c := w.cache.Load()
	if c == nil || w.cacheTTL >= 0 {
		// Either we have no cache or the cache is finite-TTL. In either case there's nothing
		// useful to write back to the store from the cache.
		return true
	}
	// Infinite-cache mode: assume the cache has a full set of current flag data (since the
	// data source has been running) and write the contents back to the underlying data store.
	kinds := datakinds.AllDataKinds()
	allData := make([]st.Collection, 0, len(kinds))
	for _, kind := range kinds {
		allCacheKey := dataStoreAllItemsCacheKey(kind)
		if data, present := c.Get(allCacheKey); present {
			if items, ok := data.([]st.KeyedItemDescriptor); ok {
				allData = append(allData, st.Collection{Kind: kind, Items: items})
			}
		}
	}
	err := w.initCore(allData)
	if err != nil {
		// initCore has already put us back into the failed state. Just log a note.
		w.loggers.Errorf("Tried to write cached data to persistent store after a store outage, but failed: %s", err)
	} else {
		w.loggers.Warn("Successfully updated persistent store from cached data")
		// Note that w.inited should have already been set when InitInternal was originally called -
		// in infinite cache mode, we set it even if the database update failed.
	}
	return true
}

// hasInfiniteCache returns true if the wrapper currently holds a cache configured with infinite TTL.
func (w *persistentDataStoreWrapper) hasInfiniteCache() bool {
	return w.cache.Load() != nil && w.cacheTTL < 0
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

// cacheCollection writes a kind's items into the given cache snapshot.
// Caller is responsible for handling a nil cache.
func cacheCollection(c *cache.Cache, kind st.DataKind, items []st.KeyedItemDescriptor) {
	copyOfItems := slices.Clone(items)
	c.Set(dataStoreAllItemsCacheKey(kind), copyOfItems, cache.DefaultExpiration)

	for _, item := range items {
		c.Set(dataStoreCacheKey(kind, item.Key), item.Item, cache.DefaultExpiration)
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

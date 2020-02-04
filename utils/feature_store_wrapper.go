// Package utils contains support code that most users of the SDK will not need to access
// directly. However, they may be useful for anyone developing custom integrations.
package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	cache "github.com/patrickmn/go-cache"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	ld "gopkg.in/launchdarkly/go-server-sdk.v5"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

// Optional interface that can be implemented by components whose types can't be easily
// determined by looking at the config object. This is also defined in diagnostic_events.go,
// but that's in another package and we'd rather not export this implementation detail.
type diagnosticsComponentDescriptor interface {
	GetDiagnosticsComponentTypeName() string
}

// UnmarshalItem attempts to unmarshal an entity that has been stored as JSON in a
// DataStore. The kind parameter indicates what type of entity is expected.
func UnmarshalItem(kind interfaces.VersionedDataKind, raw []byte) (interfaces.VersionedData, error) {
	data := kind.GetDefaultItem()
	if jsonErr := json.Unmarshal(raw, &data); jsonErr != nil {
		return nil, jsonErr
	}
	if item, ok := data.(interfaces.VersionedData); ok {
		return item, nil
	}
	return nil, fmt.Errorf("unexpected data type from JSON unmarshal: %T", data)
}

// DataStoreWrapper is a partial implementation of ldclient.DataStore that delegates basic
// functionality to an instance of DataStoreCore. It provides optional caching, and will
// automatically provide the proper data ordering when using  NonAtomicDataStoreCoreInitialization.
//
// Also, if the DataStoreCore object implements ldclient.DataStoreStatusProvider, the wrapper
// will make it possible for SDK components to react appropriately if the availability of the store
// changes (e.g. if we lose a database connection, but then regain it).
type DataStoreWrapper struct {
	core          interfaces.DataStoreCoreBase
	coreAtomic    interfaces.DataStoreCore
	coreNonAtomic interfaces.NonAtomicDataStoreCore
	coreStatus    interfaces.DataStoreCoreStatus
	statusManager *internal.DataStoreStatusManager
	cache         *cache.Cache
	requests      singleflight.Group
	loggers       ldlog.Loggers
	inited        bool
	initLock      sync.RWMutex
}

const initCheckedKey = "$initChecked"

// NewDataStoreWrapperWithConfig creates an instance of DataStoreWrapper that wraps an instance
// of DataStoreCore. It takes a Config parameter so that it can use the same logging configuration
// as the SDK.
func NewDataStoreWrapperWithConfig(core interfaces.DataStoreCore, config ld.Config) *DataStoreWrapper {
	w := newBaseWrapper(core, config)
	w.coreAtomic = core
	return w
}

// NewNonAtomicDataStoreWrapperWithConfig creates an instance of DataStoreWrapper that wraps an
// instance of NonAtomicDataStoreCore. It takes a Config parameter so that it can use the same logging configuration
// as the SDK.
func NewNonAtomicDataStoreWrapperWithConfig(core interfaces.NonAtomicDataStoreCore, config ld.Config) *DataStoreWrapper {
	w := newBaseWrapper(core, config)
	w.coreNonAtomic = core
	return w
}

// NewNonAtomicDataStoreWrapper creates an instance of DataStoreWrapper that wraps an
// instance of NonAtomicDataStoreCore.
func NewNonAtomicDataStoreWrapper(core interfaces.NonAtomicDataStoreCore) *DataStoreWrapper {
	return NewNonAtomicDataStoreWrapperWithConfig(core, ld.Config{})
}

func newBaseWrapper(core interfaces.DataStoreCoreBase, config ld.Config) *DataStoreWrapper {
	cacheTTL := core.GetCacheTTL()
	var myCache *cache.Cache
	if cacheTTL != 0 {
		myCache = cache.New(cacheTTL, 5*time.Minute)
		// Note that the documented behavior of go-cache is that if cacheTTL is negative, the
		// cache never expires. That is consistent with we've defined the parameter.
	}

	w := &DataStoreWrapper{
		core:    core,
		cache:   myCache,
		loggers: config.Loggers,
	}
	if cs, ok := core.(interfaces.DataStoreCoreStatus); ok {
		w.coreStatus = cs
	}
	w.statusManager = internal.NewDataStoreStatusManager(
		true,
		w.pollAvailabilityAfterOutage,
		myCache == nil || core.GetCacheTTL() > 0, // needsRefresh=true unless we're in infinite cache mode
		config.Loggers,
	)

	return w
}

func dataStoreCacheKey(kind interfaces.VersionedDataKind, key string) string {
	return kind.GetNamespace() + ":" + key
}

func dataStoreAllItemsCacheKey(kind interfaces.VersionedDataKind) string {
	return "all:" + kind.GetNamespace()
}

// Init performs an update of the entire data store, with optional caching.
func (w *DataStoreWrapper) Init(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
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

func (w *DataStoreWrapper) initCore(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	var err error
	if w.coreNonAtomic != nil {
		// If the store uses non-atomic initialization, we'll need to put the data in the proper update
		// order and call InitCollectionsInternal.
		colls := transformUnorderedDataToOrderedData(allData)
		err = w.coreNonAtomic.InitCollectionsInternal(colls)
	} else {
		err = w.coreAtomic.InitInternal(allData)
	}
	w.processError(err)
	return err
}

func (w *DataStoreWrapper) filterAndCacheItems(kind interfaces.VersionedDataKind, items map[string]interfaces.VersionedData) map[string]interfaces.VersionedData {
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

// Get retrieves a single item by key, with optional caching.
func (w *DataStoreWrapper) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	if w.cache == nil {
		item, err := w.core.GetInternal(kind, key)
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
		item, err := w.core.GetInternal(kind, key)
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

// All retrieves all items of the specified kind, with optional caching.
func (w *DataStoreWrapper) All(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	if w.cache == nil {
		items, err := w.core.GetAllInternal(kind)
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
		items, err := w.core.GetAllInternal(kind)
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

// Upsert updates or adds an item, with optional caching.
func (w *DataStoreWrapper) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) error {
	finalItem, err := w.core.UpsertInternal(kind, item)
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

// Delete deletes an item, with optional caching.
func (w *DataStoreWrapper) Delete(kind interfaces.VersionedDataKind, key string, version int) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return w.Upsert(kind, deletedItem)
}

// Initialized returns true if the data store contains a data set. To avoid calling the
// underlying implementation any more often than necessary (since Initialized is called often),
// DataStoreWrapper uses the following heuristic: 1. Once we have received a true result
// from InitializedInternal, we always return true. 2. If InitializedInternal returns false,
// and we have a cache, we will cache that result so we won't call it any more frequently
// than the cache TTL.
func (w *DataStoreWrapper) Initialized() bool {
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

	newValue := w.core.InitializedInternal()
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

// Close releases any resources being held by the store.
func (w *DataStoreWrapper) Close() error {
	w.statusManager.Close()
	if coreCloser, ok := w.core.(io.Closer); ok {
		return coreCloser.Close()
	}
	return nil
}

// GetStoreStatus returns the current status of the store.
func (w *DataStoreWrapper) GetStoreStatus() internal.DataStoreStatus {
	return internal.DataStoreStatus{Available: w.statusManager.IsAvailable()}
}

// StatusSubscribe creates a channel that will receive all changes in store status.
func (w *DataStoreWrapper) StatusSubscribe() internal.DataStoreStatusSubscription {
	return w.statusManager.Subscribe()
}

// Used internally to describe this component in diagnostic data.
func (w *DataStoreWrapper) GetDiagnosticsComponentTypeName() string {
	if dcd, ok := w.core.(diagnosticsComponentDescriptor); ok {
		return dcd.GetDiagnosticsComponentTypeName()
	}
	return "custom"
}

func (w *DataStoreWrapper) processError(err error) {
	if err == nil {
		// If we're waiting to recover after a failure, we'll let the polling routine take care
		// of signaling success. Even if we could signal success a little earlier based on the
		// success of whatever operation we just did, we'd rather avoid the overhead of acquiring
		// w.statusLock every time we do anything. So we'll just do nothing here.
		return
	}
	w.statusManager.UpdateAvailability(false)
}

func (w *DataStoreWrapper) pollAvailabilityAfterOutage() bool {
	if w.coreStatus == nil || !w.coreStatus.IsStoreAvailable() {
		return false
	}
	if w.hasCacheWithInfiniteTTL() {
		// If we're in infinite cache mode, then we can assume the cache has a full set of current
		// flag data (since presumably the data source has still been running) and we can just
		// write the contents of the cache to the underlying data store.
		allData := make(map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData, 2)
		for _, kind := range ld.VersionedDataKinds {
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
		}
	}
	return true
}

func (w *DataStoreWrapper) hasCacheWithInfiniteTTL() bool {
	return w.cache != nil && w.core.GetCacheTTL() < 0
}

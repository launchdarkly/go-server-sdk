// Package utils contains support code that most users of the SDK will not need to access
// directly. However, they may be useful for anyone developing custom integrations.
package utils

import (
	"time"

	cache "github.com/patrickmn/go-cache"
	ld "gopkg.in/launchdarkly/go-client.v4"
)

// FeatureStoreCore is an interface for a simplified subset of the functionality of
// ldclient.FeatureStore, to be used in conjunction with FeatureStoreWrapper. This allows
// developers of custom FeatureStore implementations to avoid repeating logic that would
// commonly be needed in any such implementation, such as caching. Instead, they can
// implement only FeatureStoreCore and then call NewFeatureStoreWrapper.
type FeatureStoreCore interface {
	// GetInternal queries a single item from the data store. The kind parameter distinguishes
	// between different categories of data (flags, segments) and the key is the unique key
	// within that category. If no such item exists, the method should return (nil, nil).
	// It should not attempt to filter out any items based on their Deleted property, nor to
	// cache any items.
	GetInternal(kind ld.VersionedDataKind, key string) (ld.VersionedData, error)
	// GetAllInternal queries all items in a given category from the data store, returning
	// a map of unique keys to items. It should not attempt to filter out any items based
	// on their Deleted property, nor to cache any items.
	GetAllInternal(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error)
	// InitInternal replaces the entire contents of the data store. It should either do
	// this atomically (if the data store supports transactions), or if that is not
	// possible, it should first add/update all items from the new data set and then
	// delete any existing keys that were not in the new data set.
	InitInternal(map[ld.VersionedDataKind]map[string]ld.VersionedData) error
	// UpsertInternal adds or updates a single item. If an item with the same key already
	// exists, it should update it only if the new item's GetVersion() value is greater
	// than the old one. It returns true if the item was updated, or false if it was not
	// updated due to the version comparison. Note that deletes are implemented by using
	// UpsertInternal to store an item whose Deleted property is true.
	UpsertInternal(kind ld.VersionedDataKind, item ld.VersionedData) (bool, error)
	// InitializedInternal returns true if the data store contains a complete data set,
	// meaning that InitInternal has been called at least once. In a shared data store, it
	// should be able to detect this even if InitInternal was called in a different process,
	// i.e. the test should be based on looking at what is in the data store. The method
	// does not need to worry about caching this value; FeatureStoreWrapper will only call
	// it when necessary.
	InitializedInternal() bool
	// GetCacheTTL returns the length of time that data should be retained in an in-memory
	// cache. This cache is maintained by FeatureStoreWrapper. If GetCacheTTL returns zero,
	// there will be no cache.
	GetCacheTTL() time.Duration
}

// FeatureStoreWrapper is a partial implementation of ldclient.FeatureStore that delegates
// basic functionality to an instance of FeatureStoreCore. It provides optional caching
type FeatureStoreWrapper struct {
	core  FeatureStoreCore
	cache *cache.Cache
}

// NewFeatureStoreWrapper creates an instance of FeatureStoreWrapper that wraps an instance
// of FeatureStoreCore.
func NewFeatureStoreWrapper(core FeatureStoreCore) *FeatureStoreWrapper {
	w := FeatureStoreWrapper{core: core}
	cacheTTL := core.GetCacheTTL()
	if cacheTTL > 0 {
		w.cache = cache.New(cacheTTL, 5*time.Minute)
	}
	return &w
}

func featureStoreCacheKey(kind ld.VersionedDataKind, key string) string {
	return kind.GetNamespace() + ":" + key
}

func featureStoreAllItemsCacheKey(kind ld.VersionedDataKind) string {
	return "all:" + kind.GetNamespace()
}

// Init performs an update of the entire data store, with optional caching.
func (w *FeatureStoreWrapper) Init(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
	if w.cache == nil {
		return w.core.InitInternal(allData)
	}
	w.cache.Flush()
	err := w.core.InitInternal(allData)
	if err != nil {
		return err
	}
	for kind, items := range allData {
		w.putAllItemsInCache(kind, items)
	}
	return nil
}

func (w *FeatureStoreWrapper) putAllItemsInCache(kind ld.VersionedDataKind, items map[string]ld.VersionedData) {
	if w.cache == nil {
		return
	}
	// We do some filtering here so that deleted items are not included in the full cached data set
	// that's used by All. This is so that All doesn't have to do that filtering itself. However,
	// since Get does know to filter out deleted items, we will still cache those individually,
	filteredItems := make(map[string]ld.VersionedData, len(items))
	for key, item := range items {
		w.cache.Set(featureStoreCacheKey(kind, key), item, cache.DefaultExpiration)
		if !item.IsDeleted() {
			filteredItems[key] = item
		}
	}
	w.cache.Set(featureStoreAllItemsCacheKey(kind), filteredItems, cache.DefaultExpiration)
}

// Get retrieves a single item by key, with optional caching.
func (w *FeatureStoreWrapper) Get(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	if w.cache == nil {
		item, err := w.core.GetInternal(kind, key)
		return itemOnlyIfNotDeleted(item), err
	}
	cacheKey := featureStoreCacheKey(kind, key)
	if data, present := w.cache.Get(cacheKey); present {
		if data == nil { // If present is true but data is nil, we have cached the absence of an item
			return nil, nil
		}
		if item, ok := data.(ld.VersionedData); ok {
			return itemOnlyIfNotDeleted(item), nil
		}
	}
	// Item was not cached or cached value was not valid
	item, err := w.core.GetInternal(kind, key)
	if err == nil {
		w.cache.Set(cacheKey, item, cache.DefaultExpiration)
	}
	return itemOnlyIfNotDeleted(item), err
}

func itemOnlyIfNotDeleted(item ld.VersionedData) ld.VersionedData {
	if item != nil && item.IsDeleted() {
		return nil
	}
	return item
}

// All retrieves all items of the specified kind, with optional caching.
func (w *FeatureStoreWrapper) All(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	if w.cache == nil {
		return w.core.GetAllInternal(kind)
	}
	// Check whether we have a cache item for the entire data set
	cacheKey := featureStoreAllItemsCacheKey(kind)
	if data, present := w.cache.Get(cacheKey); present {
		if items, ok := data.(map[string]ld.VersionedData); ok {
			return items, nil
		}
	}
	// Data set was not cached or cached value was not valid
	items, err := w.core.GetAllInternal(kind)
	if err == nil {
		w.putAllItemsInCache(kind, items)
	}
	return items, err
}

// Upsert updates or adds an item, with optional caching.
func (w *FeatureStoreWrapper) Upsert(kind ld.VersionedDataKind, item ld.VersionedData) error {
	updated, err := w.core.UpsertInternal(kind, item)
	if err == nil && updated {
		if w.cache != nil {
			w.cache.Set(featureStoreCacheKey(kind, item.GetKey()), item, cache.DefaultExpiration)
			w.cache.Delete(featureStoreAllItemsCacheKey(kind))
		}
	}
	return err
}

// Delete deletes an item, with optional caching.
func (w *FeatureStoreWrapper) Delete(kind ld.VersionedDataKind, key string, version int) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return w.Upsert(kind, deletedItem)
}

// Initialized returns true if the feature store contains a data set.
func (w *FeatureStoreWrapper) Initialized() bool {
	return w.core.InitializedInternal()
}

package ldclient

import (
	"log"
	"os"
	"sync"
	"time"

	cache "github.com/patrickmn/go-cache"
)

// FeatureStore is an interface describing a structure that maintains the live collection of features and related objects.
// It is used by LaunchDarkly when streaming mode is enabled, and stores data returned
// by the streaming API. Custom FeatureStore implementations can be passed to the
// LaunchDarkly client via a custom Config object. LaunchDarkly provides two FeatureStore
// implementations: one backed by an in-memory map, and one backed by Redis.
// Implementations must be thread-safe.
type FeatureStore interface {
	Get(kind VersionedDataKind, key string) (VersionedData, error)
	All(kind VersionedDataKind) (map[string]VersionedData, error)
	Init(map[VersionedDataKind]map[string]VersionedData) error
	Delete(kind VersionedDataKind, key string, version int) error
	Upsert(kind VersionedDataKind, item VersionedData) error
	Initialized() bool
}

// InMemoryFeatureStore is a memory based FeatureStore implementation, backed by a lock-striped map.
type InMemoryFeatureStore struct {
	allData       map[VersionedDataKind]map[string]VersionedData
	isInitialized bool
	sync.RWMutex
	logger Logger
}

// NewInMemoryFeatureStore creates a new in-memory FeatureStore instance.
func NewInMemoryFeatureStore(logger Logger) *InMemoryFeatureStore {
	if logger == nil {
		logger = log.New(os.Stderr, "[LaunchDarkly InMemoryFeatureStore]", log.LstdFlags)
	}
	return &InMemoryFeatureStore{
		allData:       make(map[VersionedDataKind]map[string]VersionedData),
		isInitialized: false,
		logger:        logger,
	}
}

// Get returns an individual object of a given type from the store
func (store *InMemoryFeatureStore) Get(kind VersionedDataKind, key string) (VersionedData, error) {
	store.RLock()
	defer store.RUnlock()
	if store.allData[kind] == nil {
		store.allData[kind] = make(map[string]VersionedData)
	}
	item := store.allData[kind][key]

	if item == nil {
		store.logger.Printf("WARN: Key: %s not found in \"%s\".", key, kind)
		return nil, nil
	} else if item.IsDeleted() {
		store.logger.Printf("WARN: Attempted to get deleted item in \"%s\". Key: %s", kind, key)
		return nil, nil
	} else {
		return item, nil
	}
}

// All returns all the objects of a given kind from the store
func (store *InMemoryFeatureStore) All(kind VersionedDataKind) (map[string]VersionedData, error) {
	store.RLock()
	defer store.RUnlock()
	ret := make(map[string]VersionedData)

	for k, v := range store.allData[kind] {
		if !v.IsDeleted() {
			ret[k] = v
		}
	}
	return ret, nil
}

// Delete removes an item of a given kind from the store
func (store *InMemoryFeatureStore) Delete(kind VersionedDataKind, key string, version int) error {
	store.Lock()
	defer store.Unlock()
	if store.allData[kind] == nil {
		store.allData[kind] = make(map[string]VersionedData)
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
func (store *InMemoryFeatureStore) Init(allData map[VersionedDataKind]map[string]VersionedData) error {
	store.Lock()
	defer store.Unlock()

	store.allData = make(map[VersionedDataKind]map[string]VersionedData)

	for k, v := range allData {
		items := make(map[string]VersionedData)
		for k1, v1 := range v {
			items[k1] = v1
		}
		store.allData[k] = items
	}

	store.isInitialized = true
	return nil
}

// Upsert inserts or replaces an item in the store unless there it already contains an item with an equal or larger version
func (store *InMemoryFeatureStore) Upsert(kind VersionedDataKind, item VersionedData) error {
	store.Lock()
	defer store.Unlock()
	if store.allData[kind] == nil {
		store.allData[kind] = make(map[string]VersionedData)
	}
	items := store.allData[kind]
	old := items[item.GetKey()]

	if old == nil || old.GetVersion() < item.GetVersion() {
		items[item.GetKey()] = item
	}
	return nil
}

// Initialized returns whether the store has been initialized with data
func (store *InMemoryFeatureStore) Initialized() bool {
	store.RLock()
	defer store.RUnlock()
	return store.isInitialized
}

// FeatureStoreHelper is a helper type that can provide caching behavior for a FeatureStore
// implementation.
type FeatureStoreHelper struct {
	cache    *cache.Cache
	cacheTTL time.Duration
}

// NewFeatureStoreHelper creates an instance of FeatureStoreHelper. If cacheTTL is
// non-zero, it will create an in-memory cache with the specified TTL and all of the
// FeatureStoreHelper methods will use this cache. If cacheTTL is zero, it will not
// create a cache and the methods will simply delegate to the underlying data store.
func NewFeatureStoreHelper(cacheTTL time.Duration) *FeatureStoreHelper {
	ret := FeatureStoreHelper{cacheTTL: cacheTTL}
	if cacheTTL > 0 {
		ret.cache = cache.New(cacheTTL, 5*time.Minute)
	}
	return &ret
}

func featureStoreCacheKey(kind VersionedDataKind, key string) string {
	return kind.GetNamespace() + ":" + key
}

func featureStoreAllItemsCacheKey(kind VersionedDataKind) string {
	return "all:" + kind.GetNamespace()
}

// Init performs an update of the entire data store, with optional caching. The uncachedInit
// function updates the underlying data.
func (fsh *FeatureStoreHelper) Init(allData map[VersionedDataKind]map[string]VersionedData,
	uncachedInit func(map[VersionedDataKind]map[string]VersionedData) error) error {
	if fsh.cache == nil {
		return uncachedInit(allData)
	}
	fsh.cache.Flush()
	err := uncachedInit(allData)
	if err != nil {
		return err
	}
	for kind, items := range allData {
		fsh.putAllItemsInCache(kind, items)
	}
	return nil
}

func (fsh *FeatureStoreHelper) putAllItemsInCache(kind VersionedDataKind, items map[string]VersionedData) {
	if fsh.cache == nil {
		return
	}
	// We do some filtering here so that deleted items are not included in the full cached data set
	// that's used by All. This is so that All doesn't have to do that filtering itself. However,
	// since Get does know to filter out deleted items, we will still cache those individually,
	filteredItems := make(map[string]VersionedData, len(items))
	for key, item := range items {
		fsh.cache.Set(featureStoreCacheKey(kind, key), item, fsh.cacheTTL)
		if !item.IsDeleted() {
			filteredItems[key] = item
		}
	}
	fsh.cache.Set(featureStoreAllItemsCacheKey(kind), filteredItems, fsh.cacheTTL)
}

// Get retrieves a single item by key, with optional caching. The uncachedGet function attempts
// to retrieve the item from the underlying data store.
func (fsh *FeatureStoreHelper) Get(kind VersionedDataKind, key string,
	uncachedGet func(VersionedDataKind, string) (VersionedData, error)) (VersionedData, error) {
	if fsh.cache == nil {
		item, err := uncachedGet(kind, key)
		return itemOnlyIfNotDeleted(item), err
	}
	cacheKey := featureStoreCacheKey(kind, key)
	if data, present := fsh.cache.Get(cacheKey); present {
		if data == nil { // If present is true but data is nil, we have cached the absence of an item
			return nil, nil
		}
		if item, ok := data.(VersionedData); ok {
			return itemOnlyIfNotDeleted(item), nil
		}
	}
	// Item was not cached or cached value was not valid
	item, err := uncachedGet(kind, key)
	if err == nil {
		fsh.cache.Set(cacheKey, item, fsh.cacheTTL)
	}
	return itemOnlyIfNotDeleted(item), err
}

func itemOnlyIfNotDeleted(item VersionedData) VersionedData {
	if item != nil && item.IsDeleted() {
		return nil
	}
	return item
}

// All retrieves all items of the specified kind, with optional caching. The uncachedAll function
// retrieves the items from the underlying data store.
func (fsh *FeatureStoreHelper) All(kind VersionedDataKind,
	uncachedAll func(VersionedDataKind) (map[string]VersionedData, error)) (map[string]VersionedData, error) {
	if fsh.cache == nil {
		return uncachedAll(kind)
	}
	// Check whether we have a cache item for the entire data set
	cacheKey := featureStoreAllItemsCacheKey(kind)
	if data, present := fsh.cache.Get(cacheKey); present {
		if items, ok := data.(map[string]VersionedData); ok {
			return items, nil
		}
	}
	// Data set was not cached or cached value was not valid
	items, err := uncachedAll(kind)
	if err == nil {
		fsh.putAllItemsInCache(kind, items)
	}
	return items, err
}

// Upsert updates or adds an item, with optional caching. The uncachedUpsert function performs
// an upsert to the underlying data store.
func (fsh *FeatureStoreHelper) Upsert(kind VersionedDataKind, item VersionedData,
	uncachedUpsert func(VersionedDataKind, VersionedData) error) error {
	if fsh.cache == nil {
		return uncachedUpsert(kind, item)
	}
	err := uncachedUpsert(kind, item)
	if err == nil {
		fsh.cache.Set(featureStoreCacheKey(kind, item.GetKey()), item, fsh.cacheTTL)
		fsh.cache.Delete(featureStoreAllItemsCacheKey(kind))
	}
	return err
}

// Delete deletes an item, with optional caching. The uncachedUpsert function performs an
// upsert to the underlying data store - Delete is implemented by upserting an item that
// is in a deleted state.
func (fsh *FeatureStoreHelper) Delete(kind VersionedDataKind, key string, version int,
	uncachedUpdate func(VersionedDataKind, VersionedData) error) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return fsh.Upsert(kind, deletedItem, uncachedUpdate)
}

package interfaces

import "time"

// DataStoreFactory is a factory that creates some implementation of DataStore.
type DataStoreFactory interface {
	CreateDataStore(context ClientContext) (DataStore, error)
}

// DataStore is an interface describing a structure that maintains the live collection of features
// and related objects. Whenever the SDK retrieves feature flag data from LaunchDarkly, via streaming
// or polling, it puts the data into the DataStore; then it queries the store whenever a flag needs
// to be evaluated. Therefore, implementations must be thread-safe.
//
// The SDK provides a default in-memory implementation (NewInMemoryDataStore), as well as database
// integrations in the "redis", "ldconsul", and "lddynamodb" packages. To use an implementation other
// than the default, put an instance of it in the DataStore property of your client configuration.
//
// If you want to create a custom implementation, it may be helpful to use the DataStoreWrapper
// type in the utils package; this provides commonly desired behaviors such as caching. Custom
// implementations must be able to handle any objects that implement the VersionedData interface,
// so if they need to marshal objects, the marshaling must be reflection-based. The VersionedDataKind
// type provides the necessary metadata to support this.
type DataStore interface {
	// Get attempts to retrieve an item of the specified kind from the data store using its unique key.
	// If no such item exists, it returns nil. If the item exists but has a Deleted property that is true,
	// it returns nil.
	Get(kind VersionedDataKind, key string) (VersionedData, error)
	// All retrieves all items of the specified kind from the data store, returning a map of keys to
	// items. Any items whose Deleted property is true must be omitted. If the store is empty, it
	// returns an empty map.
	All(kind VersionedDataKind) (map[string]VersionedData, error)
	// Init performs an update of the entire data store, replacing any existing data.
	Init(data map[VersionedDataKind]map[string]VersionedData) error
	// Delete removes the specified item from the data store, unless its Version property is greater
	// than or equal to the specified version, in which case nothing happens. Removal should be done
	// by storing an item whose Deleted property is true (use VersionedDataKind.MakeDeleteItem()).
	Delete(kind VersionedDataKind, key string, version int) error
	// Upsert adds or updates the specified item, unless the existing item in the store has a Version
	// property greater than or equal to the new item's Version, in which case nothing happens.
	Upsert(kind VersionedDataKind, item VersionedData) error
	// Initialized returns true if the data store contains a data set, meaning that Init has been
	// called at least once. In a shared data store, it should be able to detect this even if Init
	// was called in a different process, i.e. the test should be based on looking at what is in
	// the data store. Once this has been determined to be true, it can continue to return true
	// without having to check the store again; this method should be as fast as possible since it
	// may be called during feature flag evaluations.
	Initialized() bool
}

// DataStoreCoreBase defines methods that are common to the DataStoreCore and
// NonAtomicDataStoreCore interfaces.
type DataStoreCoreBase interface {
	// GetInternal queries a single item from the data store. The kind parameter distinguishes
	// between different categories of data (flags, segments) and the key is the unique key
	// within that category. If no such item exists, the method should return (nil, nil).
	// It should not attempt to filter out any items based on their Deleted property, nor to
	// cache any items.
	GetInternal(kind VersionedDataKind, key string) (VersionedData, error)
	// GetAllInternal queries all items in a given category from the data store, returning
	// a map of unique keys to items. It should not attempt to filter out any items based
	// on their Deleted property, nor to cache any items.
	GetAllInternal(kind VersionedDataKind) (map[string]VersionedData, error)
	// UpsertInternal adds or updates a single item. If an item with the same key already
	// exists, it should update it only if the new item's GetVersion() value is greater
	// than the old one. It should return the final state of the item, i.e. if the update
	// succeeded then it returns the item that was passed in, and if the update failed due
	// to the version check then it returns the item that is currently in the data store
	// (this ensures that caching works correctly).
	//
	// Note that deletes are implemented by using UpsertInternal to store an item whose
	// Deleted property is true.
	UpsertInternal(kind VersionedDataKind, item VersionedData) (VersionedData, error)
	// InitializedInternal returns true if the data store contains a complete data set,
	// meaning that InitInternal has been called at least once. In a shared data store, it
	// should be able to detect this even if InitInternal was called in a different process,
	// i.e. the test should be based on looking at what is in the data store. The method
	// does not need to worry about caching this value; DataStoreWrapper will only call
	// it when necessary.
	InitializedInternal() bool
	// GetCacheTTL returns the length of time that data should be retained in an in-memory
	// cache. This cache is maintained by DataStoreWrapper. If GetCacheTTL returns zero,
	// there will be no cache. If it returns a negative number, the cache never expires.
	GetCacheTTL() time.Duration
}

// DataStoreCoreStatus is an optional interface that can be implemented by DataStoreCoreBase
// implementations. It allows DataStoreWrapper to request a status check on the availability of
// the underlying data store.
type DataStoreCoreStatus interface {
	// Tests whether the data store seems to be functioning normally. This should not be a detailed
	// test of different kinds of operations, but just the smallest possible operation to determine
	// whether (for instance) we can reach the database. DataStoreWrapper will call this method
	// at intervals if the store has previously failed, until it returns true.
	IsStoreAvailable() bool
}

// DataStoreCore is an interface for a simplified subset of the functionality of
// DataStore, to be used in conjunction with DataStoreWrapper. This allows
// developers of custom DataStore implementations to avoid repeating logic that would
// commonly be needed in any such implementation, such as caching. Instead, they can
// implement only DataStoreCore and then call NewDataStoreWrapper.
//
// This interface assumes that the data store can update the data set atomically. If
// not, use NonAtomicDataStoreCore instead. DataStoreCoreBase defines the common methods.
type DataStoreCore interface {
	DataStoreCoreBase
	// InitInternal replaces the entire contents of the data store. This should be done
	// atomically (i.e. within a transaction).
	InitInternal(map[VersionedDataKind]map[string]VersionedData) error
}

// NonAtomicDataStoreCore is an interface for a limited subset of the functionality of
// DataStore, to be used in conjunction with DataStoreWrapper. This allows
// developers of custom DataStore implementations to avoid repeating logic that would
// commonly be needed in any such implementation, such as caching. Instead, they can
// implement only DataStoreCore and then call NewDataStoreWrapper.
//
// This interface assumes that the data store cannot update the data set atomically and
// will require the SDK to specify the order of operations. If atomic updates are possible,
// then use DataStoreCore instead. DataStoreCoreBase defines the common methods.
//
// Note that this is somewhat different from the way the LaunchDarkly SDK addresses the
// atomicity issue on most other platforms. There, the data stores just have one
// interface, which always receives the data as a map, but the SDK can control the
// iteration order of the map. That isn't possible in Go where maps never have a defined
// iteration order.
type NonAtomicDataStoreCore interface {
	DataStoreCoreBase
	// InitCollectionsInternal replaces the entire contents of the data store. The SDK will
	// pass a data set with a defined ordering; the collections (kinds) should be processed in
	// the specified order, and the items within each collection should be written in the
	// specified order. The store should delete any obsolete items only after writing all of
	// the items provided.
	InitCollectionsInternal(allData []StoreCollection) error
}

// StoreCollection is used by the NonAtomicDataStoreCore interface.
type StoreCollection struct {
	Kind  VersionedDataKind
	Items []VersionedData
}

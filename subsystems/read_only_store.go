package subsystems

import "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

// ReadOnlyStore represents a read-only data store that can be used to retrieve
// any of the SDK's supported DataKinds.
type ReadOnlyStore interface {
	// Get retrieves an item from the specified collection, if available.
	//
	// If the specified key does not exist in the collection, it should return an ItemDescriptor
	// whose Version is -1.
	//
	// If the item has been deleted and the store contains a placeholder, it should return an
	// ItemDescriptor whose Version is the version of the placeholder, and whose Item is nil.
	Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error)

	// GetAll retrieves all items from the specified collection.
	//
	// If the store contains placeholders for deleted items, it should include them in the results,
	// not filter them out.
	GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error)

	// IsInitialized returns true if the data store contains a data set, meaning that Init has been
	// called at least once.
	//
	// In a shared data store, it should be able to detect this even if Init was called in a
	// different process: that is, the test should be based on looking at what is in the data store.
	// Once this has been determined to be true, it can continue to return true without having to
	// check the store again; this method should be as fast as possible since it may be called during
	// feature flag evaluations.
	IsInitialized() bool
}

package subsystems

import "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

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

	// IsStatusMonitoringEnabled returns true if this data store implementation supports status
	// monitoring.
	//
	// This is normally only true for persistent data stores created with ldcomponents.PersistentDataStore(),
	// but it could also be true for any custom DataStore implementation that makes use of the
	// statusUpdater parameter provided to the DataStoreFactory. Returning true means that the store
	// guarantees that if it ever enters an invalid state (that is, an operation has failed or it knows
	// that operations cannot succeed at the moment), it will publish a status update, and will then
	// publish another status update once it has returned to a valid state.
	//
	// The same value will be returned from DataStoreStatusProvider.IsStatusMonitoringEnabled().
	IsStatusMonitoringEnabled() bool
}

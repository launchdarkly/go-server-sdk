package subsystems

// SynchronizersConfiguration represents the config for the primary and secondary synchronizers.
type SynchronizersConfiguration struct {
	// The synchronizer that is primarily active.
	Primary DataSynchronizer
	// A fallback synchronizer if the primary fails.
	Secondary DataSynchronizer
}

// DataSystemConfiguration represents the configuration for the data system.
type DataSystemConfiguration struct {
	// Store is the (optional) persistent data store.
	Store DataStore
	// StoreMode specifies the mode in which the persistent store should operate, if present.
	StoreMode DataStoreMode
	// Initializers obtain data for the SDK in a one-shot manner at startup. Their job is to get the SDK
	// into a state where it is serving somewhat fresh values as fast as possible.
	Initializers []DataInitializer
	// Synchronizers keep the SDK's data up-to-date continuously.
	Synchronizers SynchronizersConfiguration
}

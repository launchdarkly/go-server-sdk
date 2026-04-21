package subsystems

// SynchronizersConfiguration represents the config for synchronizers.
type SynchronizersConfiguration struct {
	// SynchronizerBuilders is an ordered list of synchronizer builders.
	// The system starts at index 0 and moves down the list on fallback or removal.
	// On recovery (when not at index 0), the system jumps back to index 0.
	SynchronizerBuilders []func() (DataSynchronizer, error)

	// FDv1FallbackBuilder is a special fallback used only when a synchronizer
	// returns FallbackToFDv1=true. When activated, the system abandons the synchronizer list
	// and switches to FDv1-only mode.
	FDv1FallbackBuilder func() (DataSynchronizer, error)
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

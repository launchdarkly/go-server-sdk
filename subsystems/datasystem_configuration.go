package subsystems

type SynchronizersConfiguration struct {
	Primary   DataSource
	Secondary DataSource
}

type StoreMode int

const (
	StoreModeRead      = 0
	StoreModeReadWrite = 1
)

type DataSystemConfiguration struct {
	Store     DataStore
	StoreMode StoreMode
	// Initializers obtain data for the SDK in a one-shot manner at startup. Their job is to get the SDK
	// into a state where it is serving somewhat fresh values as fast as possible.
	Initializers  []DataInitializer
	Synchronizers SynchronizersConfiguration
}

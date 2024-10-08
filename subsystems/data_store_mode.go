package subsystems

// DataStoreMode represents the mode of operation of a Data Store in FDV2 mode.
//
// This enum is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type DataStoreMode int

const (
	// DataStoreModeRead indicates that the data store is read-only. Data will never be written back to the store by
	// the SDK.
	DataStoreModeRead = 0
	// DataStoreModeReadWrite indicates that the data store is read-write. Data from initializers/synchronizers may be
	// written to the store as necessary.
	DataStoreModeReadWrite = 1
)

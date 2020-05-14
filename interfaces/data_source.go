package interfaces

// DataSourceFactory is a factory that creates some implementation of DataSource.
type DataSourceFactory interface {
	// CreateDataSource is called by the SDK to create the implementation instance.
	CreateDataSource(
		context ClientContext,
		dataStore DataStore,
		dataStoreStatusProvider DataStoreStatusProvider,
	) (DataSource, error)
}

// DataSource describes the interface for an object that receives feature flag data.
type DataSource interface {
	Initialized() bool
	Close() error
	Start(closeWhenReady chan<- struct{})
}

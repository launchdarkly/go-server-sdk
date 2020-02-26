package interfaces

// DataSourceFactory is a factory that creates some implementation of DataSource.
type DataSourceFactory interface {
	CreateDataSource(context ClientContext, dataStore DataStore) (DataSource, error)
}

// DataSource describes the interface for an object that receives feature flag data.
type DataSource interface {
	Initialized() bool
	Close() error
	Start(closeWhenReady chan<- struct{})
}

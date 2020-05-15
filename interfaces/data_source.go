package interfaces

// DataSourceFactory is a factory that creates some implementation of DataSource.
type DataSourceFactory interface {
	// CreateDataSource is called by the SDK to create the implementation instance.
	CreateDataSource(
		context ClientContext,
		dataSourceUpdates DataSourceUpdates,
	) (DataSource, error)
}

// DataSource describes the interface for an object that receives feature flag data.
type DataSource interface {
	// IsInitialized returns true if the data source has successfully initialized at some point.
	//
	// Once this is true, it should remain true even if a problem occurs later.
	IsInitialized() bool

	// Close permanently shuts down the data source and releases any resources it is using.
	Close() error

	// Start tells the data source to begin initializing. It should not try to make any connections
	// or do any other significant activity until Start is called.
	//
	// The data source should close the closeWhenReady channel if and when it has either successfully
	// initialized for the first time, or determined that initialization cannot ever succeed.
	Start(closeWhenReady chan<- struct{})
}

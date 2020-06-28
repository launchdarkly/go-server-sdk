package sharedtest

import "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

// SingleDataSourceFactory is a test implementation of DataSourceFactory that always returns the same
// pre-existing instance.
type SingleDataSourceFactory struct {
	Instance interfaces.DataSource
}

func (f SingleDataSourceFactory) CreateDataSource( //nolint:golint
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	return f.Instance, nil
}

// DataSourceFactoryThatExposesUpdater is a test implementation of DataSourceFactory that captures the
// DataSourceUpdates instance provided by LDClient.
type DataSourceFactoryThatExposesUpdater struct {
	UnderlyingFactory interfaces.DataSourceFactory
	DataSourceUpdates interfaces.DataSourceUpdates
}

func (f *DataSourceFactoryThatExposesUpdater) CreateDataSource( //nolint:golint
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	f.DataSourceUpdates = dataSourceUpdates
	return f.UnderlyingFactory.CreateDataSource(context, dataSourceUpdates)
}

// DataSourceFactoryWithData is a test implementation of DataSourceFactory that will cause the data
// source to provide a specific set of data when it starts.
type DataSourceFactoryWithData struct {
	Data []interfaces.StoreCollection
}

func (f DataSourceFactoryWithData) CreateDataSource( //nolint:golint
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	return &dataSourceWithData{f.Data, dataSourceUpdates, false}, nil
}

type dataSourceWithData struct {
	data              []interfaces.StoreCollection
	dataSourceUpdates interfaces.DataSourceUpdates
	inited            bool
}

func (d *dataSourceWithData) IsInitialized() bool {
	return d.inited
}

func (d *dataSourceWithData) Close() error {
	return nil
}

func (d *dataSourceWithData) Start(closeWhenReady chan<- struct{}) {
	d.dataSourceUpdates.Init(d.data)
	d.inited = true
	close(closeWhenReady)
}

// MockDataSource is a test implementation of DataSource that allows tests to control its initialization
// behavior.
type MockDataSource struct {
	Initialized bool
	CloseFn     func() error
	StartFn     func(chan<- struct{})
}

func (u MockDataSource) IsInitialized() bool { //nolint:golint
	return u.Initialized
}

func (u MockDataSource) Close() error { //nolint:golint
	if u.CloseFn == nil {
		return nil
	}
	return u.CloseFn()
}

func (u MockDataSource) Start(closeWhenReady chan<- struct{}) { //nolint:golint
	if u.StartFn == nil {
		close(closeWhenReady)
	} else {
		u.StartFn(closeWhenReady)
	}
}

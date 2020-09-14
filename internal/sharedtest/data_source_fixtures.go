package sharedtest

import (
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
)

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
	Data []ldstoretypes.Collection
}

func (f DataSourceFactoryWithData) CreateDataSource( //nolint:golint
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	return &dataSourceWithData{f.Data, dataSourceUpdates, false}, nil
}

type dataSourceWithData struct {
	data              []ldstoretypes.Collection
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

// DataSourceThatIsAlwaysInitialized returns a test DataSourceFactory that produces a data source
// that immediately reports success on startup, although it does not provide any data.
func DataSourceThatIsAlwaysInitialized() interfaces.DataSourceFactory {
	return singleDataSourceFactory{mockDataSource{Initialized: true}}
}

// DataSourceThatNeverInitializes returns a test DataSourceFactory that produces a data source
// that immediately starts up in a failed state and does not provide any data.
func DataSourceThatNeverInitializes() interfaces.DataSourceFactory {
	return singleDataSourceFactory{mockDataSource{Initialized: false}}
}

type singleDataSourceFactory struct {
	Instance interfaces.DataSource
}

func (f singleDataSourceFactory) CreateDataSource(
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	return f.Instance, nil
}

type mockDataSource struct {
	Initialized bool
	CloseFn     func() error
	StartFn     func(chan<- struct{})
}

func (u mockDataSource) IsInitialized() bool {
	return u.Initialized
}

func (u mockDataSource) Close() error {
	if u.CloseFn == nil {
		return nil
	}
	return u.CloseFn()
}

func (u mockDataSource) Start(closeWhenReady chan<- struct{}) {
	if u.StartFn == nil {
		close(closeWhenReady)
	} else {
		u.StartFn(closeWhenReady)
	}
}

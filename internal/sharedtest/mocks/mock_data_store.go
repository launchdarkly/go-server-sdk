package mocks

import (
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"
)

// DataSourceFactoryWithData is a test implementation of ComponentConfigurer that will cause the data
// source to provide a specific set of data when it starts.
type DataSourceFactoryWithData struct {
	Data []ldstoretypes.Collection
}

func (f DataSourceFactoryWithData) Build( //nolint:revive
	context subsystems.ClientContext,
) (subsystems.DataSource, error) {
	return &dataSourceWithData{f.Data, context.GetDataSourceUpdateSink(), false}, nil
}

type dataSourceWithData struct {
	data              []ldstoretypes.Collection
	dataSourceUpdates subsystems.DataSourceUpdateSink
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

// DataSourceThatIsAlwaysInitialized returns a test component factory that produces a data source
// that immediately reports success on startup, although it does not provide any data.
func DataSourceThatIsAlwaysInitialized() subsystems.ComponentConfigurer[subsystems.DataSource] {
	return SingleComponentConfigurer[subsystems.DataSource]{Instance: mockDataSource{Initialized: true}}
}

// DataSourceThatNeverInitializes returns a test component factory that produces a data source
// that immediately starts up in a failed state and does not provide any data.
func DataSourceThatNeverInitializes() subsystems.ComponentConfigurer[subsystems.DataSource] {
	return SingleComponentConfigurer[subsystems.DataSource]{Instance: mockDataSource{Initialized: false}}
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

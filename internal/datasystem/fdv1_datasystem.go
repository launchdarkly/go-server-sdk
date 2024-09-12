package datasystem

import (
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type FDv1 struct {
	dataSourceStatusBroadcaster *internal.Broadcaster[interfaces.DataSourceStatus]
	dataSourceStatusProvider    interfaces.DataSourceStatusProvider
	dataStoreStatusBroadcaster  *internal.Broadcaster[interfaces.DataStoreStatus]
	dataStoreStatusProvider     interfaces.DataStoreStatusProvider
	flagChangeEventBroadcaster  *internal.Broadcaster[interfaces.FlagChangeEvent]
	dataStore                   subsystems.DataStore
	dataSource                  subsystems.DataSource
	offline                     bool
}

func NewFDv1(offline bool, dataStoreFactory subsystems.ComponentConfigurer[subsystems.DataStore], dataSourceFactory subsystems.ComponentConfigurer[subsystems.DataSource], clientContext *internal.ClientContextImpl) (*FDv1, error) {
	system := &FDv1{
		dataSourceStatusBroadcaster: internal.NewBroadcaster[interfaces.DataSourceStatus](),
		dataStoreStatusBroadcaster:  internal.NewBroadcaster[interfaces.DataStoreStatus](),
		flagChangeEventBroadcaster:  internal.NewBroadcaster[interfaces.FlagChangeEvent](),
		offline:                     offline,
	}

	dataStoreUpdateSink := datastore.NewDataStoreUpdateSinkImpl(system.dataStoreStatusBroadcaster)
	storeFactory := dataStoreFactory
	if storeFactory == nil {
		storeFactory = ldcomponents.InMemoryDataStore()
	}
	clientContextWithDataStoreUpdateSink := clientContext
	clientContextWithDataStoreUpdateSink.DataStoreUpdateSink = dataStoreUpdateSink
	store, err := storeFactory.Build(clientContextWithDataStoreUpdateSink)
	if err != nil {
		return nil, err
	}
	system.dataStore = store

	system.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(store, dataStoreUpdateSink)

	dataSourceUpdateSink := datasource.NewDataSourceUpdateSinkImpl(
		store,
		system.dataStoreStatusProvider,
		system.dataSourceStatusBroadcaster,
		system.flagChangeEventBroadcaster,
		clientContext.GetLogging().LogDataSourceOutageAsErrorAfter,
		clientContext.GetLogging().Loggers,
	)

	dataSource, err := createDataSource(clientContext, dataSourceFactory, dataSourceUpdateSink)
	if err != nil {
		return nil, err
	}
	system.dataSource = dataSource
	system.dataSourceStatusProvider = datasource.NewDataSourceStatusProviderImpl(
		system.dataSourceStatusBroadcaster,
		dataSourceUpdateSink,
	)

	return system, nil

}

func createDataSource(
	context *internal.ClientContextImpl,
	dataSourceBuilder subsystems.ComponentConfigurer[subsystems.DataSource],
	dataSourceUpdateSink subsystems.DataSourceUpdateSink,
) (subsystems.DataSource, error) {
	if context.Offline {
		context.GetLogging().Loggers.Info("Starting LaunchDarkly client in offline mode")
		dataSourceUpdateSink.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
		return datasource.NewNullDataSource(), nil
	}
	factory := dataSourceBuilder
	if factory == nil {
		// COVERAGE: can't cause this condition in unit tests because it would try to connect to production LD
		factory = ldcomponents.StreamingDataSource()
	}
	contextCopy := *context
	contextCopy.BasicClientContext.DataSourceUpdateSink = dataSourceUpdateSink
	return factory.Build(&contextCopy)
}

func (f *FDv1) DataSourceStatusBroadcaster() *internal.Broadcaster[interfaces.DataSourceStatus] {
	return f.dataSourceStatusBroadcaster
}

func (f *FDv1) DataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	return f.dataSourceStatusProvider
}

func (f *FDv1) DataStoreStatusBroadcaster() *internal.Broadcaster[interfaces.DataStoreStatus] {
	return f.dataStoreStatusBroadcaster
}

func (f *FDv1) DataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return f.dataStoreStatusProvider
}

func (f *FDv1) FlagChangeEventBroadcaster() *internal.Broadcaster[interfaces.FlagChangeEvent] {
	return f.flagChangeEventBroadcaster
}

func (f *FDv1) Start(closeWhenReady chan struct{}) {
	f.dataSource.Start(closeWhenReady)
}

func (f *FDv1) Stop() error {
	if f.dataSource != nil {
		_ = f.dataSource.Close()
	}
	if f.dataStore != nil {
		_ = f.dataStore.Close()
	}
	if f.dataSourceStatusBroadcaster != nil {
		f.dataSourceStatusBroadcaster.Close()
	}
	if f.dataStoreStatusBroadcaster != nil {
		f.dataStoreStatusBroadcaster.Close()
	}
	if f.flagChangeEventBroadcaster != nil {
		f.flagChangeEventBroadcaster.Close()
	}
	return nil
}

func (f *FDv1) Offline() bool {
	return f.offline || f.dataSource == datasource.NewNullDataSource()
}

func (f *FDv1) DataAvailability() DataAvailability {
	if f.Offline() {
		return Defaults
	}
	if f.dataSource.IsInitialized() {
		return Refreshed
	}
	if f.dataStore.IsInitialized() {
		return Cached
	}
	return Defaults
}

func (f *FDv1) Store() subsystems.ReadOnlyStore {
	return f.dataStore
}

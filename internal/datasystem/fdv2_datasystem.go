package datasystem

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

var _ subsystems.DataDestination = (*Store)(nil)
var _ subsystems.ReadOnlyStore = (*Store)(nil)

type broadcasters struct {
	dataSourceStatus *internal.Broadcaster[interfaces.DataSourceStatus]
	dataStoreStatus  *internal.Broadcaster[interfaces.DataStoreStatus]
	flagChangeEvent  *internal.Broadcaster[interfaces.FlagChangeEvent]
}

type FDv2 struct {
	// Operates the in-memory and optional persistent store that backs data queries.
	store *Store

	// List of initializers that are capable of obtaining an initial payload of data.
	initializers []subsystems.DataInitializer

	// The primary synchronizer responsible for keeping data up-to-date.
	primarySync subsystems.DataSynchronizer

	// The secondary synchronizer, in case the primary is unavailable.
	secondarySync subsystems.DataSynchronizer

	// Whether the SDK should make use of persistent store/initializers/synchronizers or not.
	disabled bool

	loggers ldlog.Loggers

	// Cancel and wg are used to track and stop the goroutines used by the system.
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// The SDK client, via MakeClient, expects to pass a channel down into a data source which will then be
	// closed when the source is considered to be ready or in a terminal state. This is what allows the initialization
	// timeout logic to work correctly and return early - otherwise, users would have to wait the full init timeout
	// before receiving a status update. The following are true:
	// 1. Initializers may close the channel (because an initializer's job is to initialize the SDK!)
	// 2. Synchronizers may close the channel (because an initializer might not be configured, or have failed)
	// To ensure the channel is closed only once, we use a sync.Once wrapping the close() call.
	readyOnce sync.Once

	// These broadcasters are mainly to satisfy the existing SDK contract with users to provide status updates for
	// the data source, data store, and flag change events. These may be different in fdv2, but we attempt to implement
	// them for now.
	broadcasters *broadcasters

	// We hold a reference to the dataStoreStatusProvider because it's required for the public interface of the
	// SDK client.
	dataStoreStatusProvider interfaces.DataStoreStatusProvider

	dataSourceStatusProvider *dataStatusProvider
}

func NewFDv2(disabled bool, cfgBuilder subsystems.ComponentConfigurer[subsystems.DataSystemConfiguration], clientContext *internal.ClientContextImpl) (*FDv2, error) {

	store := NewStore(clientContext.GetLogging().Loggers)

	bcasters := &broadcasters{
		dataSourceStatus: internal.NewBroadcaster[interfaces.DataSourceStatus](),
		dataStoreStatus:  internal.NewBroadcaster[interfaces.DataStoreStatus](),
		flagChangeEvent:  internal.NewBroadcaster[interfaces.FlagChangeEvent](),
	}

	fdv2 := &FDv2{
		store:                    store,
		loggers:                  clientContext.GetLogging().Loggers,
		broadcasters:             bcasters,
		dataSourceStatusProvider: &dataStatusProvider{},
	}

	// Yay circular reference.
	fdv2.dataSourceStatusProvider.system = fdv2

	dataStoreUpdateSink := datastore.NewDataStoreUpdateSinkImpl(bcasters.dataStoreStatus)
	clientContextCopy := *clientContext
	clientContextCopy.DataStoreUpdateSink = dataStoreUpdateSink
	clientContextCopy.DataDestination = store
	clientContextCopy.DataSourceStatusReporter = fdv2

	cfg, err := cfgBuilder.Build(clientContextCopy)
	if err != nil {
		return nil, err
	}

	fdv2.initializers = cfg.Initializers
	fdv2.primarySync = cfg.Synchronizers.Primary
	fdv2.secondarySync = cfg.Synchronizers.Secondary
	fdv2.disabled = disabled

	if cfg.Store != nil && !disabled {
		// If there's a persistent Store, we should provide a status monitor and inform Store that it's present.
		fdv2.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(cfg.Store, dataStoreUpdateSink)
		store.WithPersistence(cfg.Store, cfg.StoreMode, fdv2.dataStoreStatusProvider)
	} else {
		// If there's no persistent Store, we still need to satisfy the SDK's public interface of having
		// a data Store status provider. So we create one that just says "I don't know what's going on".
		fdv2.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(noStatusMonitoring{}, dataStoreUpdateSink)
	}

	return fdv2, nil
}

type noStatusMonitoring struct{}

func (n noStatusMonitoring) IsStatusMonitoringEnabled() bool {
	return false
}

func (f *FDv2) Start(closeWhenReady chan struct{}) {
	if f.disabled {
		f.loggers.Infof("Data system is disabled, SDK will return application-defined default values")
		close(closeWhenReady)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.launchTask(func() {
		f.run(ctx, closeWhenReady)
	})
}

func (f *FDv2) launchTask(task func()) {
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		task()
	}()
}

func (f *FDv2) run(ctx context.Context, closeWhenReady chan struct{}) {
	payloadVersion := f.runInitializers(ctx, closeWhenReady)

	if f.dataStoreStatusProvider.IsStatusMonitoringEnabled() {
		f.launchTask(func() {
			f.runPersistentStoreOutageRecovery(ctx, f.dataStoreStatusProvider.AddStatusListener())
		})
	}

	f.runSynchronizers(ctx, closeWhenReady, payloadVersion)
}

func (f *FDv2) runPersistentStoreOutageRecovery(ctx context.Context, statuses <-chan interfaces.DataStoreStatus) {
	for {
		select {
		case newStoreStatus := <-statuses:
			if newStoreStatus.Available {
				// The Store has just transitioned from unavailable to available
				if newStoreStatus.NeedsRefresh {
					f.loggers.Warn("Reinitializing data Store from in-memory cache after after data Store outage")
					if err := f.store.Commit(); err != nil {
						f.loggers.Error("Failed to reinitialize data Store: %v", err)
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (f *FDv2) runInitializers(ctx context.Context, closeWhenReady chan struct{}) *int {
	for _, initializer := range f.initializers {
		f.loggers.Infof("Attempting to initialize via %s", initializer.Name())
		payload, err := initializer.Fetch(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			f.loggers.Warnf("Initializer %s failed: %v", initializer.Name(), err)
			continue
		}
		f.loggers.Infof("Initialized via %s", initializer.Name())
		f.store.Init(payload.Data, payload.Status)
		f.readyOnce.Do(func() {
			close(closeWhenReady)
		})
		return payload.Version
	}
	return nil
}

func (f *FDv2) runSynchronizers(ctx context.Context, closeWhenReady chan struct{}, payloadVersion *int) {
	// If the SDK was configured with no synchronizer, then (assuming no initializer succeeded), we should
	// trigger the ready signal to let the call to MakeClient unblock immediately.
	if f.primarySync == nil {
		f.readyOnce.Do(func() {
			close(closeWhenReady)
		})
		return
	}

	// We can't pass closeWhenReady to the data source, because it might have already been closed.
	// Instead, create a "proxy" channel just for the data source; if that is closed, we close the real one
	// using the sync.Once.
	ready := make(chan struct{})
	f.primarySync.Sync(ready, payloadVersion)

	for {
		select {
		case <-ready:
			f.readyOnce.Do(func() {
				close(closeWhenReady)
			})
		case <-ctx.Done():
			return
		}
	}
}

func (f *FDv2) Stop() error {
	if f.cancel != nil {
		f.cancel()
		f.wg.Wait()
	}
	_ = f.store.Close()
	if f.primarySync != nil {
		_ = f.primarySync.Close()
	}
	if f.secondarySync != nil {
		_ = f.secondarySync.Close()
	}
	return nil
}

func (f *FDv2) Store() subsystems.ReadOnlyStore {
	return f.store
}

func (f *FDv2) DataAvailability() DataAvailability {
	return f.store.DataAvailability()
}

func (f *FDv2) DataSourceStatusBroadcaster() *internal.Broadcaster[interfaces.DataSourceStatus] {
	return f.broadcasters.dataSourceStatus
}

func (f *FDv2) DataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	return f.dataSourceStatusProvider
}

func (f *FDv2) DataStoreStatusBroadcaster() *internal.Broadcaster[interfaces.DataStoreStatus] {
	return f.broadcasters.dataStoreStatus
}

func (f *FDv2) DataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return f.dataStoreStatusProvider
}

func (f *FDv2) FlagChangeEventBroadcaster() *internal.Broadcaster[interfaces.FlagChangeEvent] {
	return f.broadcasters.flagChangeEvent
}

func (f *FDv2) Offline() bool {
	return f.disabled
}

func (f *FDv2) UpdateStatus(status interfaces.DataSourceState, err interfaces.DataSourceErrorInfo) {

}

type dataStatusProvider struct {
	system *FDv2
}

func (d *dataStatusProvider) GetStatus() interfaces.DataSourceStatus {
	var state interfaces.DataSourceState
	if d.system.primarySync != nil {
		if d.system.primarySync.IsInitialized() {
			state = interfaces.DataSourceStateValid
		} else {
			state = interfaces.DataSourceStateInitializing
		}
	} else {
		state = interfaces.DataSourceStateOff
	}
	return interfaces.DataSourceStatus{
		State:      state,
		StateSince: time.Now(),
		LastError:  interfaces.DataSourceErrorInfo{},
	}
}

func (d *dataStatusProvider) AddStatusListener() <-chan interfaces.DataSourceStatus {
	return d.system.broadcasters.dataSourceStatus.AddListener()
}

func (d *dataStatusProvider) RemoveStatusListener(listener <-chan interfaces.DataSourceStatus) {
	d.system.broadcasters.dataSourceStatus.RemoveListener(listener)
}

func (d *dataStatusProvider) WaitFor(desiredState interfaces.DataSourceState, timeout time.Duration) bool {
	//TODO implement me
	panic("implement me")
}

var _ interfaces.DataSourceStatusProvider = (*dataStatusProvider)(nil)

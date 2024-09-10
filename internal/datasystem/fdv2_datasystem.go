package datasystem

import (
	"context"
	"errors"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"sync"
)

var _ subsystems.DataSourceUpdateSink = (*store)(nil)

type broadcasters struct {
	dataSourceStatus *internal.Broadcaster[interfaces.DataSourceStatus]
	dataStoreStatus  *internal.Broadcaster[interfaces.DataStoreStatus]
	flagChangeEvent  *internal.Broadcaster[interfaces.FlagChangeEvent]
}

type FDv2 struct {
	// Operates the in-memory and optional persistent store that backs data queries.
	store *store

	// List of initializers that are capable of obtaining an initial payload of data.
	initializers []subsystems.DataInitializer

	// The primary synchronizer responsible for keeping data up-to-date.
	primarySync subsystems.DataSynchronizer

	// The secondary synchronizer, in case the primary is unavailable.
	secondarySync subsystems.DataSynchronizer

	// Whether the SDK should make use of persistent store/initializers/synchronizers or not.
	offline bool

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
}

func NewFDv2(cfgBuilder subsystems.ComponentConfigurer[subsystems.DataSystemConfiguration], clientContext *internal.ClientContextImpl) (*FDv2, error) {

	store := newStore(clientContext.GetLogging().Loggers)

	bcasters := &broadcasters{
		dataSourceStatus: internal.NewBroadcaster[interfaces.DataSourceStatus](),
		dataStoreStatus:  internal.NewBroadcaster[interfaces.DataStoreStatus](),
		flagChangeEvent:  internal.NewBroadcaster[interfaces.FlagChangeEvent](),
	}

	dataStoreUpdateSink := datastore.NewDataStoreUpdateSinkImpl(bcasters.dataStoreStatus)
	clientContextCopy := *clientContext
	clientContextCopy.DataStoreUpdateSink = dataStoreUpdateSink
	clientContextCopy.DataSourceUpdateSink = store

	cfg, err := cfgBuilder.Build(clientContextCopy)
	if err != nil {
		return nil, err
	}

	fdv2 := &FDv2{
		store:         store,
		initializers:  cfg.Initializers,
		primarySync:   cfg.Synchronizers.Primary,
		secondarySync: cfg.Synchronizers.Secondary,
		offline:       cfg.Offline,
		loggers:       clientContext.GetLogging().Loggers,
		broadcasters:  bcasters,
	}

	if cfg.Store != nil {
		fdv2.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(cfg.Store, dataStoreUpdateSink)
		store.SetPersistent(cfg.Store, cfg.StoreMode, fdv2.dataStoreStatusProvider)
	} else {
		fdv2.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(noStatusMonitoring{}, dataStoreUpdateSink)
	}

	return fdv2, nil
}

type noStatusMonitoring struct{}

func (n noStatusMonitoring) IsStatusMonitoringEnabled() bool {
	return false
}

func (f *FDv2) runPersistentStoreOutageRecovery(ctx context.Context, statuses <-chan interfaces.DataStoreStatus) {
	for {
		select {
		case newStoreStatus := <-statuses:
			if newStoreStatus.Available {
				// The store has just transitioned from unavailable to available (scenario 2a above)
				if newStoreStatus.NeedsRefresh {
					f.loggers.Warn("Reinitializing data store from in-memory cache after after data store outage")
					if err := f.store.Commit(); err != nil {
						f.loggers.Error("Failed to reinitialize data store: %v", err)
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (f *FDv2) Start(closeWhenReady chan struct{}) {
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

	if f.store.Mirroring() {
		f.launchTask(func() {
			f.runPersistentStoreOutageRecovery(ctx, f.dataStoreStatusProvider.AddStatusListener())
		})
	}

	f.runSynchronizers(ctx, closeWhenReady, payloadVersion)
}

func (f *FDv2) runInitializers(ctx context.Context, closeWhenReady chan struct{}) *int {
	for _, initializer := range f.initializers {
		payload, err := initializer.Fetch(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			continue
		}
		f.store.Init(payload.Data)
		f.store.SwapToMemory(payload.Fresh)
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

	ready := make(chan struct{})
	f.primarySync.Sync(ready, payloadVersion)

	for {
		select {
		case <-ready:
			// We may have synchronizers that don't actually validate that a payload is fresh. In this case,
			// we'd need a mechanism to propagate the status to this method, just like for the initializers.
			// For now, we assume that the only synchronizers are LaunchDarkly-provided and do receive fresh payloads.
			f.store.SwapToMemory(true)
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
	return f.store.GetActive()
}

func (f *FDv2) DataStatus() DataStatus {
	if f.offline {
		return Defaults
	}
	return f.store.Status()
}

func (f *FDv2) DataSourceStatusBroadcaster() *internal.Broadcaster[interfaces.DataSourceStatus] {
	return f.broadcasters.dataSourceStatus
}

func (f *FDv2) DataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	//TODO implement me
	panic("implement me")
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
	return f.offline
}

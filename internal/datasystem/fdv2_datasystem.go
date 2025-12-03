package datasystem

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

var (
	_ subsystems.ReadOnlyStore     = (*Store)(nil)
	_ subsystems.ReadOnlyDataStore = (*Store)(nil)
)

type broadcasters struct {
	dataSourceStatus     *internal.Broadcaster[interfaces.DataSourceStatus]
	dataStoreStatus      *internal.Broadcaster[interfaces.DataStoreStatus]
	flagChangeEvent      *internal.Broadcaster[interfaces.FlagChangeEvent]
	changeSetBroadcaster *internal.Broadcaster[subsystems.ChangeSet]
}

func (b *broadcasters) Close() {
	b.dataSourceStatus.Close()
	b.dataStoreStatus.Close()
	b.flagChangeEvent.Close()
	b.changeSetBroadcaster.Close()
}

// FDv2 is an implementation of the DataSystem interface that uses the Flag Delivery V2 protocol for
// obtaining and keeping data up-to-date. Additionally, it operates with an optional persistent store
// in read-only or read/write mode.
type FDv2 struct {
	// Operates the in-memory and optional persistent store that backs data queries.
	store *Store

	// List of initializers that are capable of obtaining an initial payload of data.
	initializers []subsystems.DataInitializer

	// The primary synchronizer responsible for keeping data up-to-date.
	primarySyncBuilder func() (subsystems.DataSynchronizer, error)

	// Boolean used to track whether the datasystem was originally configured
	// with some sort of valid data source.
	//
	// We cannot check this at run time because synchronizers may be removed if
	// they permanently fail.
	configuredWithDataSources bool

	// The secondary synchronizer, in case the primary is unavailable.
	secondarySyncBuilder func() (subsystems.DataSynchronizer, error)

	// The fdv1 fallback synchronizer, in case we have to fall back to fdv1.
	fdv1SyncBuilder func() (subsystems.DataSynchronizer, error)

	// Whether the SDK should make use of persistent store/initializers/synchronizers or not.
	disabled bool

	// Whether the SDK is running in daemon mode (i.e., using Relay Proxy for data updates).
	daemonMode bool

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

	environmentIDProvider *environmentIDProvider

	// Protects status.
	mu     sync.Mutex
	status interfaces.DataSourceStatus

	fallbackCond func(status interfaces.DataSourceStatus) bool
	recoveryCond func(status interfaces.DataSourceStatus) bool
}

// NewFDv2 creates a new instance of the FDv2 data system. The first argument indicates if the system is enabled or
// disabled.
func NewFDv2(disabled bool, cfgBuilder subsystems.ComponentConfigurer[subsystems.DataSystemConfiguration],
	clientContext *internal.ClientContextImpl,
	ldRelayWrapper func(subsystems.ReadOnlyDataStore, <-chan subsystems.ChangeSet),
) (*FDv2, error) {
	bcasters := &broadcasters{
		dataSourceStatus:     internal.NewBroadcaster[interfaces.DataSourceStatus](),
		dataStoreStatus:      internal.NewBroadcaster[interfaces.DataStoreStatus](),
		flagChangeEvent:      internal.NewBroadcaster[interfaces.FlagChangeEvent](),
		changeSetBroadcaster: internal.NewBroadcaster[subsystems.ChangeSet](),
	}

	store := NewStore(clientContext.GetLogging().Loggers, bcasters.flagChangeEvent, bcasters.changeSetBroadcaster)

	if ldRelayWrapper != nil {
		ldRelayWrapper(store, bcasters.changeSetBroadcaster.AddListener())
	}

	fdv2 := &FDv2{
		store:                    store,
		loggers:                  clientContext.GetLogging().Loggers,
		broadcasters:             bcasters,
		dataSourceStatusProvider: &dataStatusProvider{},
		environmentIDProvider:    &environmentIDProvider{},
	}

	// Unfortunate circular reference.
	fdv2.dataSourceStatusProvider.system = fdv2

	dataStoreUpdateSink := datastore.NewDataStoreUpdateSinkImpl(bcasters.dataStoreStatus)
	clientContextCopy := *clientContext
	clientContextCopy.DataStoreUpdateSink = dataStoreUpdateSink
	clientContextCopy.DataSourceStatusReporter = fdv2

	cfg, err := cfgBuilder.Build(clientContextCopy)
	if err != nil {
		return nil, err
	}

	fdv2.initializers = cfg.Initializers
	fdv2.primarySyncBuilder = cfg.Synchronizers.PrimaryBuilder
	fdv2.secondarySyncBuilder = cfg.Synchronizers.SecondaryBuilder
	fdv2.fdv1SyncBuilder = cfg.Synchronizers.FDv1FallbackBuilder
	fdv2.disabled = disabled
	fdv2.fallbackCond = func(status interfaces.DataSourceStatus) bool {
		interruptedAtRuntime := status.State == interfaces.DataSourceStateInterrupted &&
			time.Since(status.StateSince) > 1*time.Minute
		cannotInitialize := status.State == interfaces.DataSourceStateInitializing &&
			time.Since(status.StateSince) > 10*time.Second

		return interruptedAtRuntime || cannotInitialize
	}
	fdv2.recoveryCond = func(status interfaces.DataSourceStatus) bool {
		interruptedAtRuntime := status.State == interfaces.DataSourceStateInterrupted &&
			time.Since(status.StateSince) > 1*time.Minute
		healthyForTooLong := status.State == interfaces.DataSourceStateValid &&
			time.Since(status.StateSince) > 5*time.Minute
		cannotInitialize := status.State == interfaces.DataSourceStateInitializing &&
			time.Since(status.StateSince) > 10*time.Second

		return interruptedAtRuntime || healthyForTooLong || cannotInitialize
	}

	fdv2.configuredWithDataSources = len(fdv2.initializers) > 0 || fdv2.primarySyncBuilder != nil
	fdv2.daemonMode = !fdv2.configuredWithDataSources && cfg.Store == nil

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

// Start starts the FDv2 data system. If not disabled, it will begin initializing via the configured
// initializers, and then start the primary synchronizer.
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
	f.UpdateStatus(interfaces.DataSourceStateInitializing, interfaces.DataSourceErrorInfo{})

	f.runInitializers(ctx, closeWhenReady)

	if f.configuredWithDataSources && f.dataStoreStatusProvider.IsStatusMonitoringEnabled() {
		f.launchTask(func() {
			f.runPersistentStoreOutageRecovery(ctx, f.dataStoreStatusProvider.AddStatusListener())
		})
	}

	f.runSynchronizers(ctx, closeWhenReady)
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

func (f *FDv2) runInitializers(ctx context.Context, closeWhenReady chan struct{}) {
	for _, initializer := range f.initializers {
		f.loggers.Infof("Attempting to initialize via %s", initializer.Name())
		basis, err := initializer.Fetch(f.store, ctx)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			f.loggers.Warnf("Initializer %s failed: %v", initializer.Name(), err)
			continue
		}
		f.environmentIDProvider.SetEnvironmentID(basis.EnvironmentID)
		f.store.Apply(basis.ChangeSet, basis.Persist)
		if basis.ChangeSet.Selector().IsDefined() {
			f.loggers.Infof("Initialized via %s", initializer.Name())
			f.readyOnce.Do(func() {
				close(closeWhenReady)
			})
			return
		}
	}
}

func (f *FDv2) runSynchronizers(ctx context.Context, closeWhenReady chan struct{}) {
	// If the SDK was configured with no synchronizer, then (assuming no initializer succeeded, which would have
	// already closed the channel), we should close it now so that MakeClient unblocks.
	if f.primarySyncBuilder == nil {
		f.readyOnce.Do(func() {
			close(closeWhenReady)
		})
		return
	}

	f.launchTask(func() {
		// Ensure we stop waiting for initialization if we exit, even if initialization fails
		defer f.readyOnce.Do(func() {
			close(closeWhenReady)
		})

		for {
			primarySync, err := f.primarySyncBuilder()
			if err != nil {
				f.loggers.Errorf("Failed to build the primary synchronizer: %v", err)
				return
			}

			f.loggers.Debugf("Primary synchronizer %s is starting", primarySync.Name())
			resultChan := primarySync.Sync(f.store)
			removeSync, fallbackv1, err := f.consumeSynchronizerResults(ctx, resultChan, f.fallbackCond, closeWhenReady)
			if errors.Is(err, context.Canceled) {
				return
			} else if err := primarySync.Close(); err != nil {
				f.loggers.Errorf("Primary synchronizer %s failed to gracefully close: %v", primarySync.Name(), err)
			}

			if removeSync {
				f.primarySyncBuilder = f.secondarySyncBuilder
				f.secondarySyncBuilder = nil

				if fallbackv1 {
					f.primarySyncBuilder = f.fdv1SyncBuilder
				}

				if f.primarySyncBuilder == nil {
					f.loggers.Debugf("No more synchronizers available, closing the channel")
					f.UpdateStatus(interfaces.DataSourceStateOff, f.getStatus().LastError)
					f.readyOnce.Do(func() {
						close(closeWhenReady)
					})
					return
				}
			} else {
				f.loggers.Debugf("Fallback condition met")
			}

			if f.secondarySyncBuilder == nil {
				continue
			}

			secondarySync, err := f.secondarySyncBuilder()
			if err != nil {
				f.loggers.Errorf("Failed to build the secondary synchronizer: %v", err)
				return
			}
			f.loggers.Debugf("Secondary synchronizer %s is starting", secondarySync.Name())

			resultChan = secondarySync.Sync(f.store)
			removeSync, fallbackv1, err = f.consumeSynchronizerResults(ctx, resultChan, f.recoveryCond, closeWhenReady)
			if errors.Is(err, context.Canceled) {
				return
			} else if err := secondarySync.Close(); err != nil {
				f.loggers.Errorf("Secondary synchronizer %s failed to gracefully close: %v", secondarySync.Name(), err)
			}

			if removeSync {
				f.secondarySyncBuilder = nil

				if fallbackv1 {
					f.primarySyncBuilder = f.fdv1SyncBuilder

					if f.primarySyncBuilder == nil {
						f.loggers.Debugf("No more synchronizers available, closing the channel")
						f.UpdateStatus(interfaces.DataSourceStateOff, f.getStatus().LastError)
						f.readyOnce.Do(func() {
							close(closeWhenReady)
						})
						return
					}
				}
			}

			f.loggers.Debugf("Recovery condition met")
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	})
}

func (f *FDv2) consumeSynchronizerResults(
	ctx context.Context,
	resultChan <-chan subsystems.DataSynchronizerResult,
	cond func(status interfaces.DataSourceStatus) bool,
	closeWhenReady chan<- struct{},
) (bool, bool, error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, false, ctx.Err()
		case result, ok := <-resultChan:
			// The status channel being closed means that we won't be receiving
			// any more information from that synchronizer and we should
			// probably fall back.
			if !ok {
				return false, false, nil
			}

			if result.EnvironmentID.IsDefined() {
				f.environmentIDProvider.SetEnvironmentID(result.EnvironmentID)
			}

			switch result.State {
			case interfaces.DataSourceStateValid:
				if result.ChangeSet != nil {
					f.store.Apply(*result.ChangeSet, true)
				}

				f.readyOnce.Do(func() {
					close(closeWhenReady)
				})

				f.UpdateStatus(result.State, result.Error)
			case interfaces.DataSourceStateInterrupted:
				f.UpdateStatus(result.State, result.Error)
			case interfaces.DataSourceStateOff:
				f.UpdateStatus(interfaces.DataSourceStateInterrupted, result.Error)
				return true, result.RevertToFDv1, nil
			}
		case <-ticker.C:
			status := f.getStatus()
			f.loggers.Debugf("Data source status used to evaluate condition: %s", status.String())
			if cond(status) {
				return false, false, nil
			}
			f.loggers.Debugf("Condition check succeeded, continue with current synchronizer")
		}
	}
}

// Stop shuts down the data system. It will close any active synchronizers. If initialization is in progress,
// it will cancel the process gracefully.
func (f *FDv2) Stop() error {
	if f.cancel != nil {
		f.cancel()
	}
	f.wg.Wait()
	_ = f.store.Close()
	f.broadcasters.Close()

	return nil
}

//nolint:revive // DataSystem method.
func (f *FDv2) Store() subsystems.ReadOnlyStore {
	return f.store
}

//nolint:revive // DataSystem method.
func (f *FDv2) DataAvailability() DataAvailability {
	if f.store.Selector().IsDefined() {
		return Refreshed
	}

	if f.store.IsInitialized() {
		return Cached
	}

	return Defaults
}

//nolint:revive // DataSystem method.
func (f *FDv2) TargetAvailability() DataAvailability {
	if f.disabled {
		return Defaults
	}

	if f.configuredWithDataSources {
		return Refreshed
	}

	if f.daemonMode {
		return Cached
	}

	return Defaults
}

//nolint:revive // DataSystem method.
func (f *FDv2) DataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	return f.dataSourceStatusProvider
}

//nolint:revive // DataSystem method.
func (f *FDv2) DataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return f.dataStoreStatusProvider
}

//nolint:revive // DataSystem method.
func (f *FDv2) EnvironmentIDProvider() internal.EnvironmentIDProvider {
	return f.environmentIDProvider
}

//nolint:revive // DataSystem method.
func (f *FDv2) FlagChangeEventBroadcaster() *internal.Broadcaster[interfaces.FlagChangeEvent] {
	return f.broadcasters.flagChangeEvent
}

//nolint:revive // DataSystem method.
func (f *FDv2) Offline() bool {
	return f.disabled
}

//nolint:revive // DataSourceStatusReporter method.
func (f *FDv2) UpdateStatus(state interfaces.DataSourceState, err interfaces.DataSourceErrorInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()

	changed := false
	if state != f.status.State {
		f.status.State = state
		f.status.StateSince = time.Now()
		changed = true
	}

	if err != f.status.LastError {
		f.status.LastError = err
		changed = true
	}

	if changed {
		f.broadcasters.dataSourceStatus.Broadcast(f.status)
	}
}

func (f *FDv2) getStatus() interfaces.DataSourceStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

type dataStatusProvider struct {
	system *FDv2
}

func (d *dataStatusProvider) GetStatus() interfaces.DataSourceStatus {
	return d.system.getStatus()
}

func (d *dataStatusProvider) AddStatusListener() <-chan interfaces.DataSourceStatus {
	return d.system.broadcasters.dataSourceStatus.AddListener()
}

func (d *dataStatusProvider) RemoveStatusListener(listener <-chan interfaces.DataSourceStatus) {
	d.system.broadcasters.dataSourceStatus.RemoveListener(listener)
}

func (d *dataStatusProvider) WaitFor(desiredState interfaces.DataSourceState, timeout time.Duration) bool {
	ch := d.AddStatusListener()
	defer d.RemoveStatusListener(ch)

	switch d.system.getStatus().State {
	case desiredState:
		return true
	case interfaces.DataSourceStateOff:
		return false
	}

	deadline := time.After(timeout)

	for {
		select {
		case status := <-ch:
			if status.State == desiredState {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

var _ interfaces.DataSourceStatusProvider = (*dataStatusProvider)(nil)

type environmentIDProvider struct {
	environmentID ldvalue.OptionalString
}

func (e environmentIDProvider) GetEnvironmentID() ldvalue.OptionalString {
	return e.environmentID
}

func (e *environmentIDProvider) SetEnvironmentID(environmentID ldvalue.OptionalString) {
	e.environmentID = environmentID
}

var _ internal.EnvironmentIDProvider = (*environmentIDProvider)(nil)

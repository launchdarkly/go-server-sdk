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

	// Mutable list of synchronizer builders. Items are removed when they permanently fail.
	// When falling back to FDv1, this list is replaced with a single FDv1 synchronizer.
	synchronizerBuilders []func() (subsystems.DataSynchronizer, error)
	currentSyncIndex     int

	// FDv1 fallback builder, used only when a synchronizer requests fallback to FDv1
	fdv1FallbackBuilder func() (subsystems.DataSynchronizer, error)

	// Boolean used to track whether the datasystem was originally configured
	// with some sort of valid data source.
	//
	// We cannot check this at run time because synchronizers may be removed if
	// they permanently fail.
	configuredWithDataSources bool

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

	// Set when a data source provides data that is applied to the store. This drives
	// InitializationSucceeded. Data already present in a persistent store does not set it.
	dataApplied internal.AtomicBoolean

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
	fdv2.synchronizerBuilders = cfg.Synchronizers.SynchronizerBuilders
	fdv2.currentSyncIndex = 0
	fdv2.fdv1FallbackBuilder = cfg.Synchronizers.FDv1FallbackBuilder
	fdv2.disabled = disabled

	fdv2.fallbackCond = func(status interfaces.DataSourceStatus) bool {
		interruptedAtRuntime := status.State == interfaces.DataSourceStateInterrupted &&
			time.Since(status.StateSince) > 1*time.Minute
		cannotInitialize := status.State == interfaces.DataSourceStateInitializing &&
			time.Since(status.StateSince) > 10*time.Second

		return interruptedAtRuntime || cannotInitialize
	}
	fdv2.recoveryCond = func(status interfaces.DataSourceStatus) bool {
		healthyForTooLong := status.State == interfaces.DataSourceStateValid &&
			time.Since(status.StateSince) > 5*time.Minute

		return healthyForTooLong
	}

	fdv2.configuredWithDataSources = len(fdv2.initializers) > 0 || len(fdv2.synchronizerBuilders) > 0

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

	if fallback, errorInfo := f.runInitializers(ctx, closeWhenReady); fallback {
		if f.fdv1FallbackBuilder != nil {
			f.loggers.Warn("Falling back to FDv1 protocol")
			f.synchronizerBuilders = []func() (subsystems.DataSynchronizer, error){f.fdv1FallbackBuilder}
			f.currentSyncIndex = 0
		} else {
			f.loggers.Warn("Initializer requested FDv1 fallback but none configured")
			f.synchronizerBuilders = nil
			f.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)
		}
	}

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

// runInitializers runs each configured initializer in order, stopping early when one provides a
// basis with a selector, the context is cancelled, or an initializer signals a fallback to FDv1.
// A basis without a selector is applied to the store and the loop continues to the next
// initializer.
//
// Once all initializers have run, initialization is complete if any initializer's data was
// applied to the store, even if that data had no selector. A basis that carries no data, or data
// that cannot be applied, does not count; the synchronizers then decide readiness.
//
// fallbackToFDv1 is true when an initializer asked the SDK to switch to FDv1. If the fallback is
// signalled alongside a basis, that basis is applied before returning, so evaluations can serve
// the server-provided data while the FDv1 synchronizer spins up. If any initializer's data was
// applied by then, initialization is complete before returning.
//
// errorInfo describes the underlying error for status reporting when no FDv1 fallback is
// configured. It is empty when the fallback accompanied a successful response.
func (f *FDv2) runInitializers(
	ctx context.Context, closeWhenReady chan struct{},
) (fallbackToFDv1 bool, errorInfo interfaces.DataSourceErrorInfo) {
	// Name of the last initializer whose data was applied to the store.
	appliedFrom := ""
	for _, initializer := range f.initializers {
		f.loggers.Infof("Attempting to initialize via %s", initializer.Name())
		basis, fallback, err := initializer.Fetch(f.store, ctx)
		if errors.Is(err, context.Canceled) {
			return false, interfaces.DataSourceErrorInfo{}
		}
		if fallback {
			if err != nil {
				f.loggers.Warnf("Initializer %s requested fallback to FDv1 protocol: %v", initializer.Name(), err)
				errorInfo = interfaces.DataSourceErrorInfo{
					Kind:    interfaces.DataSourceErrorKindUnknown,
					Message: err.Error(),
					Time:    time.Now(),
				}
			} else {
				f.loggers.Warnf("Initializer %s requested fallback to FDv1 protocol", initializer.Name())
			}
			if basis != nil && f.applyBasis(*basis, initializer.Name()) {
				appliedFrom = initializer.Name()
			}
			if appliedFrom != "" {
				f.loggers.Infof("Initialized via %s before falling back to FDv1", appliedFrom)
				f.completeInitialization(closeWhenReady)
			}
			return true, errorInfo
		}
		if err != nil {
			f.loggers.Warnf("Initializer %s failed: %v", initializer.Name(), err)
			continue
		}
		if !f.applyBasis(*basis, initializer.Name()) {
			continue
		}
		appliedFrom = initializer.Name()
		if basis.ChangeSet.Selector().IsDefined() {
			f.loggers.Infof("Initialized via %s", initializer.Name())
			f.completeInitialization(closeWhenReady)
			return false, interfaces.DataSourceErrorInfo{}
		}
	}
	if appliedFrom != "" {
		f.loggers.Infof("Initialized via %s; the data has no selector", appliedFrom)
		f.completeInitialization(closeWhenReady)
	}
	return false, interfaces.DataSourceErrorInfo{}
}

// applyBasis applies an initializer's basis to the store. It reports whether the store received
// data. A basis with no data, or with data that cannot be applied, is reported as false.
func (f *FDv2) applyBasis(basis subsystems.Basis, initializerName string) bool {
	f.environmentIDProvider.SetEnvironmentID(basis.EnvironmentID)
	if !f.store.Apply(basis.ChangeSet, basis.Persist) {
		f.loggers.Warnf("Initializer %s returned no usable data", initializerName)
		return false
	}
	f.dataApplied.Set(true)
	return true
}

// completeInitialization marks initialization as complete from the initializer phase. The status
// update comes before the readiness signal, so a caller that wakes on the signal observes the
// valid status.
func (f *FDv2) completeInitialization(closeWhenReady chan struct{}) {
	f.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
	f.readyOnce.Do(func() {
		close(closeWhenReady)
	})
}

func (f *FDv2) runSynchronizers(ctx context.Context, closeWhenReady chan struct{}) {
	// If no synchronizers configured, close ready channel and return
	if len(f.synchronizerBuilders) == 0 {
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
			// Check if we've run out of synchronizers
			if len(f.synchronizerBuilders) == 0 {
				f.loggers.Warn("No more synchronizers available")
				f.UpdateStatus(interfaces.DataSourceStateOff, f.getStatus().LastError)
				return
			}

			// Ensure currentSyncIndex is within bounds (shouldn't happen with proper logic)
			if f.currentSyncIndex >= len(f.synchronizerBuilders) {
				f.currentSyncIndex = 0
			}

			// Build synchronizer
			sync, err := f.synchronizerBuilders[f.currentSyncIndex]()
			if err != nil {
				f.loggers.Errorf("Failed to build synchronizer at index %d: %v", f.currentSyncIndex, err)
				// Remove the failed builder from the list
				f.synchronizerBuilders = append(
					f.synchronizerBuilders[:f.currentSyncIndex],
					f.synchronizerBuilders[f.currentSyncIndex+1:]...)
				// Don't increment currentSyncIndex - it now points to the next synchronizer
				continue
			}

			f.loggers.Infof("Synchronizer at index %d (%s) is starting", f.currentSyncIndex, sync.Name())
			resultChan := sync.Sync(f.store)
			action, err := f.consumeSynchronizerResults(ctx, resultChan, closeWhenReady)

			if err := sync.Close(); err != nil {
				f.loggers.Errorf("Synchronizer %s failed to close: %v", sync.Name(), err)
			}

			if errors.Is(err, context.Canceled) {
				return
			}

			// Handle action based on conditions
			switch action {
			case syncFDv1:
				if f.fdv1FallbackBuilder != nil {
					f.loggers.Warn("Falling back to FDv1 protocol")
					// Replace entire list with single FDv1 synchronizer
					f.synchronizerBuilders = []func() (subsystems.DataSynchronizer, error){f.fdv1FallbackBuilder}
					f.currentSyncIndex = 0
					continue
				}
				f.loggers.Warn("Synchronizer requested FDv1 fallback but none configured")
				f.UpdateStatus(interfaces.DataSourceStateOff, f.getStatus().LastError)
				return
			case syncRemove:
				f.loggers.Warnf("Permanently removing synchronizer at index %d", f.currentSyncIndex)
				f.synchronizerBuilders = append(
					f.synchronizerBuilders[:f.currentSyncIndex],
					f.synchronizerBuilders[f.currentSyncIndex+1:]...)
				// Don't increment currentSyncIndex - it now points to the next synchronizer
				continue
			case syncRecover:
				// Recovery: jump back to index 0
				f.loggers.Info("Recovery condition met, returning to first synchronizer")
				f.currentSyncIndex = 0
			case syncFallback:
				// Fallback: move to next index
				f.loggers.Info("Fallback condition met, trying next synchronizer")
				f.currentSyncIndex++
			}

			// Check for cancellation before next iteration
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	})
}

type syncAction int

const (
	syncFallback syncAction = iota
	syncRecover
	syncRemove
	syncFDv1
)

func (f *FDv2) consumeSynchronizerResults(
	ctx context.Context,
	resultChan <-chan subsystems.DataSynchronizerResult,
	closeWhenReady chan<- struct{},
) (action syncAction, err error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return syncFallback, ctx.Err()
		case result, ok := <-resultChan:
			// The status channel being closed means that we won't be receiving
			// any more information from that synchronizer and we should
			// probably fall back.
			if !ok {
				return syncFallback, nil
			}

			if result.EnvironmentID.IsDefined() {
				f.environmentIDProvider.SetEnvironmentID(result.EnvironmentID)
			}

			switch result.State {
			case interfaces.DataSourceStateValid:
				if result.ChangeSet != nil && f.store.Apply(*result.ChangeSet, true) {
					f.dataApplied.Set(true)
				}

				// Report the valid state before the readiness signal. A caller that wakes on
				// the signal must observe the updated status.
				f.UpdateStatus(result.State, result.Error)

				f.readyOnce.Do(func() {
					close(closeWhenReady)
				})
			case interfaces.DataSourceStateInterrupted:
				f.UpdateStatus(result.State, result.Error)
			case interfaces.DataSourceStateOff:
				f.UpdateStatus(interfaces.DataSourceStateInterrupted, result.Error)
				if result.FallbackToFDv1 {
					return syncFDv1, nil
				}
				return syncRemove, nil
			}

			// FallbackToFDv1 may ride along on a Valid or Interrupted result too -- e.g. a
			// successful response whose headers also requested the fallback. The Valid/
			// Interrupted branches above already applied any ChangeSet and updated status;
			// now hand control to the FDv1 fallback synchronizer.
			if result.FallbackToFDv1 {
				return syncFDv1, nil
			}
		case <-ticker.C:
			// If there's only one synchronizer, don't check conditions
			if len(f.synchronizerBuilders) == 1 {
				continue
			}

			status := f.getStatus()
			f.loggers.Debugf("Data source status used to evaluate condition: %s", status.String())

			// Check fallback condition first (things are bad)
			if f.fallbackCond(status) {
				f.loggers.Debugf("Fallback condition met")
				return syncFallback, nil
			}

			// If not at index 0, also check recovery condition (things are good)
			if f.currentSyncIndex > 0 && f.recoveryCond(status) {
				f.loggers.Debugf("Recovery condition met")
				return syncRecover, nil
			}

			f.loggers.Debugf("No condition met, continue with current synchronizer")
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

	if !f.configuredWithDataSources || f.store.IsInitialized() {
		return Cached
	}

	return Defaults
}

//nolint:revive // DataSystem method.
func (f *FDv2) InitializationSucceeded() bool {
	// Initialization requires that a data source provided data that was applied to the store.
	// Data already present in a persistent store does not count: a store is not a data source.
	// That data still counts as available for evaluations and for DataAvailability.
	// A configuration with no data sources cannot receive data, so it is successful as-is.
	if !f.configuredWithDataSources {
		return true
	}
	return f.dataApplied.Get()
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

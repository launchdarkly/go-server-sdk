package datasystem

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"sync"
)

// Store is a hybrid persistent/in-memory store that serves queries for data from the evaluation
// algorithm.
//
// At any given moment, 1 of 2 stores is active: in-memory, or persistent. This doesn't preclude a caller
// from holding on to a reference to the persistent store even when we swap to the in-memory store.
//
// Once the in-memory store has data (either from initializers running, or from a synchronizer), the persistent
// store is no longer regarded as active. From that point forward, GetActive() will return the in-memory store.
//
// The idea is that persistent stores can offer a way to immediately start evaluating flags before a connection
// is made to LD (or even in a very brief moment before an initializer has run.) The persistent store has caching
// logic which can result in inconsistent/stale date being used. Therefore, once we have fresh data, we don't
// want to use the persistent store at all.
//
// A complication is that persistent stores have historically operated in multiple regimes. The first is "daemon mode",
// where the SDK is effectively using the store in read-only mode, with the store being populated by Relay or another SDK.
// The second is just plain persistent store mode, where it is both read and written to. In the FDv2 system, we explicitly
// differentiate these cases using a read/read-write mode. In all cases, the in-memory store is used once it has data available.
// This contrasts from FDv1 where even if data from LD is available, that data may fall out of memory due to the persistent
// store's caching logic ("sparse mode", when the TTL is non-infinite).
//
// We have found this to almost always be undesirable for users.
type Store struct {
	// Represents a remote store, like Redis. This is optional; if present, it's only used
	// before the in-memory store is initialized.
	persistentStore subsystems.DataStore

	// The persistentStore is read-only, or read-write. In read-only mode, the store
	// is *never* written to, and only read before the in-memory store is initialized.
	// This is equivalent to the concept of "daemon mode".
	//
	// In read-write mode, data from initializers/synchronizers is written to the store
	// as it is received. This is equivalent to the normal "persistent store" configuration
	// that an SDK can use to collaborate with zero or more other SDKs with a (possibly shared) database.
	persistentStoreMode subsystems.StoreMode

	// This exists as a quirk of the DataSourceUpdateSink interface, which store implements. The DataSourceUpdateSink
	// has a method to return a DataStoreStatusProvider so that a DataSource can monitor the state of the store. This
	// was originally used in fdv1 to know when the store went offline/online, so that data could be committed back
	// to the store when it came back online. In fdv2 system, this is handled by the FDv2 struct itself, so the
	// data source doesn't need any knowledge of it. We can delete this piece of infrastructure when we no longer
	// need to support fdv1 (or we could refactor the fdv2 data sources to use a different set of interfaces that don't
	// require this.)
	persistentStoreStatusProvider interfaces.DataStoreStatusProvider

	// Represents the store that all flag/segment data queries are served from after data is received from
	// initializers/synchronizers. Before the in-memory store is initialized, queries are served from the
	// persistentStore (if configured).
	memoryStore subsystems.DataStore

	active subsystems.DataStore

	// Whether the memoryStore's data should be considered authoritative, or fresh - that is, if it is known
	// to be the latest data. Data from a baked in file for example would not be considered refreshed. The purpose
	// of this is to know if we should commit data to the persistentStore. For example, if we initialize with "stale"
	// data from a local file (refreshed=false), we may not want to pollute a connected Redis database with it.
	// TODO: this could also be called "Authoritative". "It was the latest at some point.. that point being when we asked
	// if it was the latest".
	refreshed bool

	// Protects the memory and refreshed fields.
	mu sync.RWMutex

	loggers ldlog.Loggers
}

// NewStore creates a new store. By default the store is in-memory. To add a persistent store, call SwapToPersistent. Ensure this is
// called at configuration time, only once and before the store is ever accessed.
func NewStore(loggers ldlog.Loggers) *Store {
	s := &Store{
		persistentStore:     nil,
		persistentStoreMode: subsystems.StoreModeRead,
		memoryStore:         datastore.NewInMemoryDataStore(loggers),
		refreshed:           false,
		loggers:             loggers,
	}
	s.SwapToMemory(false)
	return s
}

// Close closes the store. If there is a persistent store configured, it will be closed.
func (s *Store) Close() error {
	if s.persistentStore != nil {
		return s.persistentStore.Close()
	}
	return nil
}

// GetActive returns the active store, either persistent or in-memory. If there is no persistent store configured,
// the in-memory store is always active.
func (s *Store) getActive() subsystems.DataStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// DataStatus returns the status of the store's data. Defaults means there is no data, Cached means there is
// data, but it's not guaranteed to be recent, and Refreshed means the data has been refreshed from the server.
func (s *Store) DataStatus() DataStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.active.IsInitialized() {
		if s.refreshed {
			return Refreshed
		}
		return Cached
	}
	return Defaults
}

// Mirroring returns true data is being mirrored to a persistent store.
func (s *Store) Mirroring() bool {
	return s.persistentStore != nil && s.persistentStoreMode == subsystems.StoreModeReadWrite
}

// nolint:revive // Standard DataSourceUpdateSink method
func (s *Store) Init(allData []ldstoretypes.Collection) bool {
	// TXNS-PS: Requirement 1.3.3, must apply updates to in-memory before the persistent Store.
	// TODO: handle errors from initializing the memory or persistent stores.
	_ = s.memoryStore.Init(allData)

	if s.Mirroring() {
		_ = s.persistentStore.Init(allData) // TODO: insert in topo-sort order
	}
	return true
}

// nolint:revive // Standard DataSourceUpdateSink method
func (s *Store) Upsert(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) bool {
	var (
		memErr  error
		persErr error
	)

	// TXNS-PS: Requirement 1.3.3, must apply updates to in-memory before the persistent store.
	_, memErr = s.memoryStore.Upsert(kind, key, item)

	if s.Mirroring() {
		_, persErr = s.persistentStore.Upsert(kind, key, item)
	}
	return memErr == nil && persErr == nil
}

// nolint:revive // Standard DataSourceUpdateSink method
func (s *Store) UpdateStatus(newState interfaces.DataSourceState, newError interfaces.DataSourceErrorInfo) {
	//TODO: although DataSourceUpdateSink is where data is pushed to the store by the data source, it doesn't really
	// make sense to have it also be the place that status updates are received. It only cares whether data has
	// *ever* been received, and that is already known by the store.
	// This should probably be refactored so that the data source takes a separate injected dependency for the
	// status updates.
	s.loggers.Info("fdv2_store: swallowing status update (", newState, ", ", newError, ")")
}

// nolint:revive // Standard DataSourceUpdateSink method
func (s *Store) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return s.persistentStoreStatusProvider
}

// SwapToPersistent exists only because of the weird way the Go SDK is configured - we need a ClientContext
// before we can call Build to actually get the persistent store. That ClientContext requires the
// DataStoreUpdateSink, which is what this store struct implements.
func (s *Store) SwapToPersistent(persistent subsystems.DataStore, mode subsystems.StoreMode, statusProvider interfaces.DataStoreStatusProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persistentStore = persistent
	s.persistentStoreMode = mode
	s.persistentStoreStatusProvider = statusProvider
	s.active = s.persistentStore
	s.refreshed = false
}

func (s *Store) SwapToMemory(isRefreshed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshed = isRefreshed
	s.active = s.memoryStore
}

func (s *Store) Commit() error {
	if s.DataStatus() == Refreshed && s.Mirroring() {
		flags, err := s.memoryStore.GetAll(datakinds.Features)
		if err != nil {
			return err
		}
		segments, err := s.memoryStore.GetAll(datakinds.Segments)
		if err != nil {
			return err
		}
		return s.persistentStore.Init([]ldstoretypes.Collection{
			{Kind: datakinds.Features, Items: flags},
			{Kind: datakinds.Segments, Items: segments},
		})
	}
	return nil
}

func (s *Store) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	return s.getActive().GetAll(kind)
}

func (s *Store) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	return s.getActive().Get(kind, key)
}

func (s *Store) IsInitialized() bool {
	return s.getActive().IsInitialized()
}

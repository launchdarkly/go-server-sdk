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

type store struct {
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

	// Whether the memoryStore is active or not. This should go from false -> true and never back.
	memory bool

	// Whether the memoryStore's data should be considered authoritative, or fresh - that is, if it is known
	// to be the latest data. Data from a baked in file for example would not be considered refreshed. The purpose
	// of this is to know if we should commit data to the persistentStore. For example, if we initialize with "stale"
	// data from a local file (refreshed=false), we may not want to pollute a connected Redis database with it.
	refreshed bool

	// Protects the memory and refreshed fields.
	mu sync.RWMutex

	loggers ldlog.Loggers
}

func newStore(loggers ldlog.Loggers) *store {
	return &store{
		persistentStore:     nil,
		persistentStoreMode: subsystems.StoreModeRead,
		memoryStore:         datastore.NewInMemoryDataStore(loggers),
		memory:              true,
		loggers:             loggers,
	}
}

// This method exists only because of the weird way the Go SDK is configured - we need a ClientContext
// before we can call Build to actually get ther persistent store. That ClientContext requires the
// DataStoreUpdateSink, which is what this store struct implements.
func (s *store) SetPersistent(persistent subsystems.DataStore, mode subsystems.StoreMode, statusProvider interfaces.DataStoreStatusProvider) {
	s.persistentStore = persistent
	s.persistentStoreMode = mode
	s.memory = false
}

func (s *store) Close() error {
	if s.persistentStore != nil {
		return s.persistentStore.Close()
	}
	return nil
}

func (s *store) GetActive() subsystems.DataStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.memory {
		return s.memoryStore
	}
	return s.persistentStore
}

func (s *store) Status() DataStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// The logic here is:
	// 1. If the memory store is active, we either got that data from an (initializer|synchronizer) that indicated
	// the data is the latest known (Refreshed) or that it is potentially stale (Cached). This is set when SwapToMemory
	// is called.
	// 2. Otherwise, the persistent store - if any - is active. If there is none configured, the status is Defaults.
	//   If there is, we need to query the database availability to determine if we actually have access to the data
	//   or not.
	if s.memory {
		if s.refreshed {
			return Refreshed
		}
		return Cached
	}
	if s.persistentStore != nil {
		if s.persistentStore.IsInitialized() {
			return Cached
		}
	}
	return Defaults

}

func (s *store) Mirroring() bool {
	return s.persistentStore != nil && s.persistentStoreMode == subsystems.StoreModeReadWrite
}

func (s *store) Init(allData []ldstoretypes.Collection) bool {
	// TXNS-PS: Requirement 1.3.3, must apply updates to in-memory before the persistent store.
	// TODO: handle errors from initializing the memory or persistent stores.
	_ = s.memoryStore.Init(allData)

	if s.Mirroring() {
		_ = s.persistentStore.Init(allData) // TODO: insert in topo-sort order
	}
	return true
}

func (s *store) Upsert(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) bool {
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

func (s *store) UpdateStatus(newState interfaces.DataSourceState, newError interfaces.DataSourceErrorInfo) {
	//TODO: In the FDv2 world, instead of having users check the state, we instead have them monitor the
	// DataStatus(), because that's actually what they care about.
	// For now, discard any status updates coming from the data sources.
	s.loggers.Info("fdv2_store: swallowing status update (", newState, ", ", newError, ")")
}

func (s *store) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return s.persistentStoreStatusProvider
}

func (s *store) SwapToMemory(isRefreshed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memory = true
	s.refreshed = isRefreshed
}

func (s *store) Commit() error {
	if s.Status() == Refreshed && s.Mirroring() {
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

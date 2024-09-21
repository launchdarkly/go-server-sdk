package datasystem

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
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
	// Represents the SDK's source of truth for flag evals before initialization, or permanently if there are
	// no initializers/synchronizers configured. This is option; if not defined, only the memoryStore is used.
	persistentStore *persistentStore

	// Represents the SDK's source of truth for flag evaluations (once initialized). Before initialization,
	// the persistentStore may be used if configured.
	memoryStore *datastore.MemoryStore

	// True if the data in the memory store may be persisted to the persistent store.
	persist bool

	// Points to the active store. Swapped upon initialization.
	active subsystems.DataStore

	// Identifies the current data set.
	selector fdv2proto.Selector

	mu sync.RWMutex

	loggers ldlog.Loggers
}

type persistentStore struct {
	impl subsystems.DataStore
	// The persistentStore is read-only, or read-write. In read-only mode, the store
	// is *never* written to, and only read before the in-memory store is initialized.
	// This is equivalent to the concept of "daemon mode".
	//
	// In read-write mode, data from initializers/synchronizers is written to the store
	// as it is received. This is equivalent to the normal "persistent store" configuration
	// that an SDK can use to collaborate with zero or more other SDKs with a (possibly shared) database.
	mode subsystems.DataStoreMode
	// This exists as a quirk of the DataSourceUpdateSink interface, which store implements. The DataSourceUpdateSink
	// has a method to return a DataStoreStatusProvider so that a DataSource can monitor the state of the store. This
	// was originally used in fdv1 to know when the store went offline/online, so that data could be committed back
	// to the store when it came back online. In fdv2 system, this is handled by the FDv2 struct itself, so the
	// data source doesn't need any knowledge of it. We can delete this piece of infrastructure when we no longer
	// need to support fdv1 (or we could refactor the fdv2 data sources to use a different set of interfaces that don't
	// require this.)
	statusProvider interfaces.DataStoreStatusProvider
}

// NewStore creates a new store. By default the store is in-memory. To add a persistent store, call SwapToPersistent. Ensure this is
// called at configuration time, only once and before the store is ever accessed.
func NewStore(loggers ldlog.Loggers) *Store {
	s := &Store{
		persistentStore: nil,
		memoryStore:     datastore.NewInMemoryDataStore(loggers),
		loggers:         loggers,
		selector:        fdv2proto.NoSelector(),
		persist:         false,
	}
	s.active = s.memoryStore
	return s
}

func (s *Store) Selector() fdv2proto.Selector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selector
}

// Close closes the store. If there is a persistent store configured, it will be closed.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.persistentStore != nil {
		return s.persistentStore.impl.Close()
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

func (s *Store) shouldPersist() bool {
	return s.persist && s.persistentStore != nil && s.persistentStore.mode == subsystems.DataStoreModeReadWrite
}

func (s *Store) init(allData []ldstoretypes.Collection, selector fdv2proto.Selector, persist bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.memoryStore.SetBasis(allData)

	s.persist = persist
	s.selector = selector

	s.active = s.memoryStore

	if s.shouldPersist() {
		return s.persistentStore.impl.Init(allData) // TODO: insert in dependency order
	}

	return nil
}

func (s *Store) SetBasis(events []fdv2proto.Event, selector fdv2proto.Selector, persist bool) error {
	collections := fdv2proto.ToStorableItems(events)
	return s.init(collections, selector, persist)
}

func (s *Store) ApplyDelta(events []fdv2proto.Event, selector fdv2proto.Selector, persist bool) error {
	collections := fdv2proto.ToStorableItems(events)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.memoryStore.ApplyDelta(collections)

	s.persist = persist
	s.selector = selector

	// The process for applying the delta to the memory store is different than the persistent store
	// because persistent stores are not yet transactional in regards to payload version. This means
	// we still need to apply a series of upserts, so the state of the store may be inconsistent when that
	// is happening. In practice, we often don't receive more than one event at a time, but this may change
	// in the future.
	if s.shouldPersist() {
		for _, event := range events {
			var err error
			switch e := event.(type) {
			case fdv2proto.PutObject:
				_, err = s.persistentStore.impl.Upsert(e.Kind, e.Key, ldstoretypes.ItemDescriptor{Version: e.Version, Item: e.Object})
			case fdv2proto.DeleteObject:
				_, err = s.persistentStore.impl.Upsert(e.Kind, e.Key, ldstoretypes.ItemDescriptor{Version: e.Version, Item: nil})
			}
			// TODO: return error?
			if err != nil {
				s.loggers.Errorf("Error applying %s to persistent store: %s", event.Name(), err)
			}
		}
	}

	return nil
}

// GetDataStoreStatusProvider returns the status provider for the persistent store, if one is configured, otherwise
// nil.
func (s *Store) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.persistentStore == nil {
		return nil
	}
	return s.persistentStore.statusProvider
}

// WithPersistence exists only because of the way the SDK's configuration builders work - we need a ClientContext
// before we can call Build to actually get the persistent store. That ClientContext requires the
// DataStoreUpdateSink, which is what this store struct implements.
func (s *Store) WithPersistence(persistent subsystems.DataStore, mode subsystems.DataStoreMode, statusProvider interfaces.DataStoreStatusProvider) *Store {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.persistentStore = &persistentStore{
		impl:           persistent,
		mode:           mode,
		statusProvider: statusProvider,
	}

	s.active = s.persistentStore.impl
	return s
}

func (s *Store) Commit() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.shouldPersist() {
		flags, err := s.memoryStore.GetAll(datakinds.Features)
		if err != nil {
			return err
		}
		segments, err := s.memoryStore.GetAll(datakinds.Segments)
		if err != nil {
			return err
		}
		return s.persistentStore.impl.Init([]ldstoretypes.Collection{
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

package datasystem

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/toposort"

	"github.com/launchdarkly/go-server-sdk/v7/internal/memorystorev2"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// Store is a dual-mode persistent/in-memory store that serves requests for data from the evaluation
// algorithm.
//
// At any given moment one of two stores is active: in-memory, or persistent. This doesn't preclude a caller
// from holding on to a reference to the persistent store even when we swap to the in-memory store.
//
// Once the in-memory store has data (either from initializers or a synchronizer), the persistent
// store is no longer read from. From that point forward, calls to Get will serve data from the memory
// store.
//
// One motivation behind using persistent stores is to offer a way to immediately start evaluating
// flags before a connection is made to LD (or even in the moment before an initializer has run).
//
// The persistent store has TTL caching logic which can result in inconsistent/stale date being returned. Therefore,
// once we have fresh data from LD, we don't want to use the persistent store for reads any longer.
//
// One complication is that persistent stores have historically operated in multiple regimes. The first, "daemon mode",
// is when the SDK is effectively using the store in read-only mode, with the store being populated by Relay
// or another SDK.
//
// The second is normal persistent store mode, where it is both read and written to. In the FDv2 system, we explicitly
// differentiate these cases using a read/read-write mode switch. In all cases, the in-memory store is used once it
// has data available, although in read-write mode the persistent store may still be written to when data updates
// arrive, even though the memory store is serving reads.
//
// This contrasts from FDv1 where even if data from LD is available, that data may fall out of memory due to the
// persistent store's caching logic ("sparse mode", when the TTL is non-infinite). This was because the SDK's main Store
// implementation was a wrapper around the persistent store, rather than entirely independent.
//
// We have found the previous behavior to almost always be undesirable for users. By keeping the persistent and memory
// stores distinct, it should be much clearer where exactly data is coming from and the behavior should be more
// predictable at runtime.
type Store struct {
	// Source of truth for flag evals (before initialization), or permanently if there are
	// no initializers/synchronizers configured. Optional; if not defined, only memoryStore is used.
	persistentStore *persistentStore

	// Source of truth for flag evaluations (once initialized). Before initialization,
	// the persistentStore may be used if configured.
	memoryStore *memorystorev2.Store

	// Used to track dependencies between items in the store. This helps ensure
	// we trigger the correct flag change notifications.
	dependencyTracker *dependencyTracker

	// Broadcaster for flag change events.
	flagChangeEvent *internal.Broadcaster[interfaces.FlagChangeEvent]

	// True if the data in the memory store may be persisted to the persistent store.
	//
	// This may be false if an initializer/synchronizer has received data that shouldn't propagate memory to the
	// persistent store, such as another database or untrusted file.
	//
	// Generally only LD data sources should request data persistence.
	persist bool

	// Points to the active store. Swapped upon initialization.
	active subsystems.ReadOnlyStore

	// Identifies the current data.
	selector fdv2proto.Selector

	mu sync.RWMutex

	loggers ldlog.Loggers
}

type persistentStore struct {
	// Contains the actual store implementation.
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

func (p *persistentStore) writable() bool {
	return p != nil && p.mode == subsystems.DataStoreModeReadWrite
}

// NewStore creates a new store. If a persistent store needs to be configured, call WithPersistence before any other
// method is called.
func NewStore(
	loggers ldlog.Loggers,
	flagChangeEvent *internal.Broadcaster[interfaces.FlagChangeEvent],
) *Store {
	s := &Store{
		persistentStore:   nil,
		memoryStore:       memorystorev2.New(loggers),
		dependencyTracker: newDependencyTracker(),
		flagChangeEvent:   flagChangeEvent,
		loggers:           loggers,
		selector:          fdv2proto.NoSelector(),
		persist:           false,
	}
	s.active = s.memoryStore
	return s
}

// WithPersistence exists to accommodate the SDK's configuration builders. We need a ClientContext
// before we can call Build to actually get the persistent store. That ClientContext requires the
// DataDestination, which is what this store struct implements. Therefore, the call to NewStore and
// WithPersistence need to be separate.
func (s *Store) WithPersistence(persistent subsystems.DataStore, mode subsystems.DataStoreMode,
	statusProvider interfaces.DataStoreStatusProvider) *Store {
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

// Selector returns the current selector.
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

// SetBasis sets the basis of the store. Any existing data is discarded. To request data persistence,
// set persist to true.
func (s *Store) SetBasis(events []fdv2proto.Change, selector fdv2proto.Selector, persist bool) {
	collections, err := fdv2proto.ToStorableItems(events)
	if err != nil {
		s.loggers.Errorf("store: couldn't set basis due to malformed data: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var prechangeSnapshot map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor
	// Because flag listeners can be added and removed at any time, we need to
	// check at a single point in time if there are listeners. If so, we can
	// take a snapshot of the existing data.
	//
	// Later, instead of checking if there are listeners, we can check if we have a snapshot.
	if s.flagChangeEvent.HasListeners() {
		prechangeSnapshot = make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor)
		for _, kind := range datakinds.AllDataKinds() {
			if items, err := s.memoryStore.GetAll(kind); err == nil {
				m := make(map[string]ldstoretypes.ItemDescriptor)
				for _, item := range items {
					m[item.Key] = item.Item
				}
				prechangeSnapshot[kind] = m
			} else {
				s.loggers.Errorf("store: couldn't get all items of kind %s: %v", kind, err)
			}
		}
	}

	s.memoryStore.SetBasis(collections)

	s.updateDependencyTrackerFromFullDataSet(collections)
	// If we took a snapshot, we had change listeners interested at the time.
	// So try to notify them. If no listeners are attached now, it won't matter
	// as they will be skipped.
	if prechangeSnapshot != nil {
		s.sendChangeEvents(s.computeChangedItemsForFullDataSet(prechangeSnapshot, fullDataSetToMap(collections)))
	}

	s.persist = persist
	s.selector = selector

	s.active = s.memoryStore

	if s.shouldPersist() {
		//nolint:godox
		// TODO: figure out where to handle/report the error.
		_ = s.persistentStore.impl.Init(toposort.Sort(collections))
	}
}

func (s *Store) shouldPersist() bool {
	return s.persist && s.persistentStore.writable()
}

// ApplyDelta applies a delta update to the store. ApplyDelta should not be called until SetBasis has been called.
// To request data persistence, set persist to true.
func (s *Store) ApplyDelta(events []fdv2proto.Change, selector fdv2proto.Selector, persist bool) {
	collections, err := fdv2proto.ToStorableItems(events)
	if err != nil {
		s.loggers.Errorf("store: couldn't apply delta due to malformed data: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.memoryStore.ApplyDelta(collections)

	hasListeners := s.flagChangeEvent.HasListeners()
	affectedItems := make(toposort.Neighbors)
	for _, collection := range collections {
		for _, item := range collection.Items {
			s.dependencyTracker.updateDependenciesFrom(collection.Kind, item.Key, item.Item)
			if hasListeners {
				s.dependencyTracker.addAffectedItems(affectedItems, toposort.NewVertex(collection.Kind, item.Key))
			}
		}
	}

	if len(affectedItems) > 0 {
		s.sendChangeEvents(affectedItems)
	}

	s.persist = persist
	s.selector = selector

	// The process for applying the delta to the memory store is different than the persistent store
	// because persistent stores are not yet transactional in regards to payload version. This means
	// we still need to apply a series of upserts, so the state of the store may be inconsistent when that
	// is happening. In practice, we often don't receive more than one event at a time, but this may change
	// in the future.
	if s.shouldPersist() {
		//nolint:godox
		// TODO: figure out where to handle/report the error.
		for _, coll := range toposort.Sort(collections) {
			for _, item := range coll.Items {
				_, err := s.persistentStore.impl.Upsert(coll.Kind, item.Key, item.Item)
				if err != nil {
					s.loggers.Errorf("Failed to apply delta to persistent store: %s", err)
				}
			}
		}
	}
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

// Commit persists the data in the memory store to the persistent store, if configured. The persistent store
// must also be in write mode, and the last call to SetBasis or ApplyDelta must have had persist set to true.
func (s *Store) Commit() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.shouldPersist() {
		return s.persistentStore.impl.Init(s.memoryStore.GetAllKinds())
	}
	return nil
}

func (s *Store) getActive() subsystems.ReadOnlyStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

//nolint:revive // Implementation for ReadOnlyStore.
func (s *Store) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	return s.getActive().GetAll(kind)
}

//nolint:revive // Implementation for ReadOnlyStore.
func (s *Store) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	return s.getActive().Get(kind, key)
}

//nolint:revive // Implementation for ReadOnlyStore.
func (s *Store) IsInitialized() bool {
	return s.getActive().IsInitialized()
}

// updateDependencyTrackerFromFullDataSet is a copy of the implementation in
// internal.datasource. This duplication will be removed when FDv1 support is
// removed.
func (s *Store) updateDependencyTrackerFromFullDataSet(allData []ldstoretypes.Collection) {
	s.dependencyTracker.reset()
	for _, coll := range allData {
		for _, item := range coll.Items {
			s.dependencyTracker.updateDependenciesFrom(coll.Kind, item.Key, item.Item)
		}
	}
}

// sendChangeEvents is a copy of the implementation in internal.datasource.
// This duplication will be removed when FDv1 support is removed.
func (s *Store) sendChangeEvents(affectedItems toposort.Neighbors) {
	for item := range affectedItems {
		if item.Kind() == datakinds.Features {
			s.flagChangeEvent.Broadcast(interfaces.FlagChangeEvent{Key: item.Key()})
		}
	}
}

// computeChangedItemsForFullDataSet is a copy of the implementation in
// internal.datasource. This duplication will be removed when FDv1 support is
// removed.
func (s *Store) computeChangedItemsForFullDataSet(
	oldDataMap map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor,
	newDataMap map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor,
) toposort.Neighbors {
	affectedItems := make(toposort.Neighbors)
	for _, kind := range datakinds.AllDataKinds() {
		oldItems := oldDataMap[kind]
		newItems := newDataMap[kind]
		allKeys := make([]string, 0, len(oldItems)+len(newItems))
		for key := range oldItems {
			allKeys = append(allKeys, key)
		}
		for key := range newItems {
			if _, found := oldItems[key]; !found {
				allKeys = append(allKeys, key)
			}
		}
		for _, key := range allKeys {
			_, haveOld := oldItems[key]
			_, haveNew := newItems[key]
			if haveOld || haveNew {
				if !haveOld || !haveNew {
					s.dependencyTracker.addAffectedItems(affectedItems, toposort.NewVertex(kind, key))
				}
			}
		}
	}
	return affectedItems
}

// fullDataSetToMap is a copy of the implementation in internal.datasource.
// This duplication will be removed when FDv1 support is removed.
func fullDataSetToMap(
	allData []ldstoretypes.Collection,
) map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor {
	ret := make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor, len(allData))
	for _, coll := range allData {
		m := make(map[string]ldstoretypes.ItemDescriptor, len(coll.Items))
		for _, item := range coll.Items {
			m[item.Key] = item.Item
		}
		ret[coll.Kind] = m
	}
	return ret
}

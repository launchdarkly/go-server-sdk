// Package memorystorev2 contains an implementation for a transactional memory store suitable
// for the FDv2 architecture.
package memorystorev2

import (
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// Store provides an abstraction that makes flag and segment data available to other components.
// It accepts updates in batches - for instance, flag A was upserted while segment B was deleted -
// such that the contents of the store are consistent with a single payload version at any given time.
//
// The terminology used is "basis" and "deltas". First, the store's basis is set. This is this initial
// data, upon which subsequent deltas will be applied. Whenever the basis is set, any existing data
// is discarded.
//
// Deltas are then applied to the store. A single delta update transforms the contents of the store
// atomically. The idea is that there's never a moment when the state of the store could be inconsistent
// with regard to the authoritative LaunchDarkly SaaS.
type Store struct {
	allData       map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor
	isInitialized bool
	sync.RWMutex
	loggers ldlog.Loggers
}

// New creates a new Store. The Store is uninitialized until SetBasis is called.
func New(loggers ldlog.Loggers) *Store {
	return &Store{
		allData:       make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor),
		isInitialized: false,
		loggers:       loggers,
	}
}

// SetBasis sets the basis of the Store. Any existing data is discarded.
// When the basis is set, the store becomes initialized.
func (s *Store) SetBasis(allData []ldstoretypes.Collection) {
	s.Lock()
	defer s.Unlock()

	s.allData = make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor)

	for _, coll := range allData {
		items := make(map[string]ldstoretypes.ItemDescriptor)
		for _, item := range coll.Items {
			items[item.Key] = item.Item
		}
		s.allData[coll.Kind] = items
	}

	s.isInitialized = true
}

// ApplyDelta applies a delta update to the store. ApplyDelta should not be called until
// SetBasis has been called at least once. The return value indicates, for each DataKind
// present in the delta, whether the item in the delta was actually updated or not.
//
// An item is updated only if the version of the item in the delta is greater than the version
// in the store, or it wasn't already present.
func (s *Store) ApplyDelta(allData []ldstoretypes.Collection) map[ldstoretypes.DataKind]map[string]bool {
	updatedMap := make(map[ldstoretypes.DataKind]map[string]bool)

	s.Lock()
	defer s.Unlock()

	for _, coll := range allData {
		for _, item := range coll.Items {
			updated := s.upsert(coll.Kind, item.Key, item.Item)
			if updatedMap[coll.Kind] == nil {
				updatedMap[coll.Kind] = make(map[string]bool)
			}
			updatedMap[coll.Kind][item.Key] = updated
		}
	}

	return updatedMap
}

// Get retrieves an item of the specified kind from the store. If the item is not found, then
// ItemDescriptor{}.NotFound() is returned with a nil error.
func (s *Store) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	s.RLock()

	var item ldstoretypes.ItemDescriptor
	coll, ok := s.allData[kind]
	if ok {
		item, ok = coll[key]
	}

	s.RUnlock()

	if ok {
		return item, nil
	}
	if s.loggers.IsDebugEnabled() {
		s.loggers.Debugf(`Key %s not found in "%s"`, key, kind)
	}
	return ldstoretypes.ItemDescriptor{}.NotFound(), nil
}

// GetAll retrieves all items of the specified kind from the store.
func (s *Store) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	s.RLock()
	defer s.RUnlock()
	return s.getAll(kind), nil
}

func (s *Store) getAll(kind ldstoretypes.DataKind) []ldstoretypes.KeyedItemDescriptor {
	var itemsOut []ldstoretypes.KeyedItemDescriptor
	if itemsMap, ok := s.allData[kind]; ok {
		if len(itemsMap) > 0 {
			itemsOut = make([]ldstoretypes.KeyedItemDescriptor, 0, len(itemsMap))
			for key, item := range itemsMap {
				itemsOut = append(itemsOut, ldstoretypes.KeyedItemDescriptor{Key: key, Item: item})
			}
		}
	}
	return itemsOut
}

// GetAllKinds retrieves all items of all kinds from the store. This is different from calling
// GetAll for each kind because it provides a consistent view at a single point in time.
func (s *Store) GetAllKinds() []ldstoretypes.Collection {
	s.RLock()
	defer s.RUnlock()

	allData := make([]ldstoretypes.Collection, 0, len(s.allData))
	for kind := range s.allData {
		itemsOut := s.getAll(kind)
		allData = append(allData, ldstoretypes.Collection{Kind: kind, Items: itemsOut})
	}

	return allData
}

func (s *Store) upsert(
	kind ldstoretypes.DataKind,
	key string,
	newItem ldstoretypes.ItemDescriptor) bool {
	var coll map[string]ldstoretypes.ItemDescriptor
	var ok bool
	shouldUpdate := true
	updated := false
	if coll, ok = s.allData[kind]; ok {
		if item, ok := coll[key]; ok {
			if item.Version >= newItem.Version {
				shouldUpdate = false
			}
		}
	} else {
		s.allData[kind] = map[string]ldstoretypes.ItemDescriptor{key: newItem}
		shouldUpdate = false // because we already initialized the map with the new item
		updated = true
	}
	if shouldUpdate {
		coll[key] = newItem
		updated = true
	}
	return updated
}

// IsInitialized returns true if the store has been initialized with a basis.
func (s *Store) IsInitialized() bool {
	s.RLock()
	defer s.RUnlock()
	return s.isInitialized
}

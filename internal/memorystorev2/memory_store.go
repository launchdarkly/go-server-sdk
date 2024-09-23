package memorystorev2

import (
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// Store contains flag and segment data, protected by a lock-striped map.
//
// Implementation notes:
//
// We deliberately do not use a defer pattern to manage the lock in these methods. Using defer adds a small but
// consistent overhead, and these store methods may be called with very high frequency (at least in the case of
// Get and IsInitialized). To make it safe to hold a lock without deferring the unlock, we must ensure that
// there is only one return point from each method, and that there is no operation that could possibly cause a
// panic after the lock has been acquired. See notes on performance in CONTRIBUTING.md.
type Store struct {
	allData       map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor
	isInitialized bool
	sync.RWMutex
	loggers ldlog.Loggers
}

// New creates an instance of the in-memory data s. This is not part of the public API.
func New(loggers ldlog.Loggers) *Store {
	return &Store{
		allData:       make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor),
		isInitialized: false,
		loggers:       loggers,
	}
}

func (s *Store) SetBasis(allData []ldstoretypes.Collection) {
	s.Lock()

	s.allData = make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor)

	for _, coll := range allData {
		items := make(map[string]ldstoretypes.ItemDescriptor)
		for _, item := range coll.Items {
			items[item.Key] = item.Item
		}
		s.allData[coll.Kind] = items
	}

	s.isInitialized = true

	s.Unlock()
}

func (s *Store) ApplyDelta(allData []ldstoretypes.Collection) map[ldstoretypes.DataKind]map[string]bool {

	updatedMap := make(map[ldstoretypes.DataKind]map[string]bool)

	s.Lock()

	for _, coll := range allData {
		for _, item := range coll.Items {
			updated := s.upsert(coll.Kind, item.Key, item.Item)
			if updatedMap[coll.Kind] == nil {
				updatedMap[coll.Kind] = make(map[string]bool)
			}
			updatedMap[coll.Kind][item.Key] = updated
		}
	}

	s.Unlock()

	return updatedMap
}

func (s *Store) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	s.RLock()

	var coll map[string]ldstoretypes.ItemDescriptor
	var item ldstoretypes.ItemDescriptor
	var ok bool
	coll, ok = s.allData[kind]
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

func (s *Store) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	s.RLock()

	itemsOut := s.getAll(kind)

	s.RUnlock()

	return itemsOut, nil
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

func (s *Store) GetAllKinds() []ldstoretypes.Collection {
	s.RLock()

	var allData []ldstoretypes.Collection
	for kind := range s.allData {
		itemsOut := s.getAll(kind)
		allData = append(allData, ldstoretypes.Collection{Kind: kind, Items: itemsOut})
	}

	s.RUnlock()

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

func (s *Store) IsInitialized() bool {
	s.RLock()
	ret := s.isInitialized
	s.RUnlock()
	return ret
}

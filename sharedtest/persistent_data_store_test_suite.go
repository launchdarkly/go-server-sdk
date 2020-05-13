package sharedtest

import (
	"testing"

	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PersistentDataStoreTestSuite provides a configurable test suite for all implementations of
// PersistentDataStore.
//
// In order to be testable with this tool, a data store implementation must have the following
// characteristics:
//
// 1. It has some notion of a "prefix" string that can be used to distinguish between different
// SDK instances using the same underlying database.
//
// 2. Two instances of the same data store type with the same configuration, and the same prefix,
// should be able to see each other's data.
type PersistentDataStoreTestSuite struct {
	storeFactoryFn               func(string) intf.PersistentDataStoreFactory
	clearDataFn                  func(string) error
	concurrentModificationHookFn func(store intf.PersistentDataStore, hook func())
}

// NewPersistentDataStoreTestSuite creates a PersistentDataStoreTestSuite for testing some
// implementation of PersistentDataStore.
//
// The storeFactoryFn parameter is a function that takes a prefix string and returns a configured
// factory for this data store type (for instance, ldconsul.DataStore().Prefix(prefix)). If the
// prefix string is "", it should use the default prefix defined by the data store implementation.
// The factory must include any necessary configuration that may be appropriate for the test
// environment (for instance, pointing it to a database instance that has been set up for the
// tests).
//
// The clearDataFn parameter is a function that takes a prefix string and deletes any existing
// data that may exist in the database corresponding to that prefix.
func NewPersistentDataStoreTestSuite(
	storeFactoryFn func(prefix string) intf.PersistentDataStoreFactory,
	clearDataFn func(prefix string) error,
) *PersistentDataStoreTestSuite {
	return &PersistentDataStoreTestSuite{
		storeFactoryFn: storeFactoryFn,
		clearDataFn:    clearDataFn,
	}
}

func (s *PersistentDataStoreTestSuite) ConcurrentModificationHook(
	setHookFn func(store intf.PersistentDataStore, hook func()),
) *PersistentDataStoreTestSuite {
	s.concurrentModificationHookFn = setHookFn
	return s
}

// Run runs the configured test suite.
func (s *PersistentDataStoreTestSuite) Run(t *testing.T) {
	t.Run("Init", s.runInitTests)
	t.Run("Get", s.runGetTests)
	t.Run("Upsert", s.runUpsertTests)
	t.Run("Delete", s.runDeleteTests)

	t.Run("IsStoreAvailable", func(t *testing.T) {
		// The store should always be available during this test suite
		s.withDefaultStore(func(store intf.PersistentDataStore) {
			assert.True(t, store.IsStoreAvailable())
		})
	})

	t.Run("prefix independence", s.runPrefixIndependenceTests)
	t.Run("concurrent modification", s.runConcurrentModificationTests)
}

func (s *PersistentDataStoreTestSuite) makeStore(prefix string) intf.PersistentDataStore {
	store, err := s.storeFactoryFn(prefix).CreatePersistentDataStore(stubClientContext{})
	if err != nil {
		panic(err)
	}
	return store
}

func (s *PersistentDataStoreTestSuite) clearData(prefix string) {
	err := s.clearDataFn(prefix)
	if err != nil {
		panic(err)
	}
}

func (s *PersistentDataStoreTestSuite) initWithEmptyData(store intf.PersistentDataStore) {
	err := store.Init(MakeMockDataSet())
	if err != nil {
		panic(err)
	}
}

func (s *PersistentDataStoreTestSuite) withDefaultStore(action func(intf.PersistentDataStore)) {
	store := s.makeStore("")
	defer store.Close()
	action(store)
}

func (s *PersistentDataStoreTestSuite) withDefaultInitedStore(action func(intf.PersistentDataStore)) {
	s.clearData("")
	store := s.makeStore("")
	defer store.Close()
	s.initWithEmptyData(store)
	action(store)
}

func (s *PersistentDataStoreTestSuite) runInitTests(t *testing.T) {
	t.Run("store initialized after init", func(t *testing.T) {
		s.clearData("")
		store := s.makeStore("")
		item1 := &MockDataItem{Key: "feature"}
		allData := MakeMockDataSet(item1)
		require.NoError(t, store.Init(allData))

		assert.True(t, store.IsInitialized())
	})

	t.Run("completely replaces previous data", func(t *testing.T) {
		s.clearData("")
		s.withDefaultStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "first", Version: 1}
			item2 := &MockDataItem{Key: "second", Version: 1}
			otherItem1 := &MockDataItem{Key: "first", Version: 1, IsOtherKind: true}
			allData := MakeMockDataSet(item1, item2, otherItem1)
			require.NoError(t, store.Init(allData))

			items, err := store.GetAll(MockData)
			require.NoError(t, err)
			assert.Len(t, items, 2)
			assert.Equal(t, item1, items[item1.Key])
			assert.Equal(t, item2, items[item2.Key])

			otherItems, err := store.GetAll(MockOtherData)
			require.NoError(t, err)
			assert.Len(t, otherItems, 1)
			assert.Equal(t, otherItem1, otherItems[otherItem1.Key])

			otherItem2 := &MockDataItem{Key: "second", Version: 1, IsOtherKind: true}
			allData = MakeMockDataSet(item1, otherItem2)
			require.NoError(t, store.Init(allData))

			items, err = store.GetAll(MockData)
			require.NoError(t, err)
			assert.Len(t, items, 1)
			assert.Equal(t, item1, items[item1.Key])

			otherItems, err = store.GetAll(MockOtherData)
			require.NoError(t, err)
			assert.Len(t, otherItems, 1)
			assert.Equal(t, otherItem2, otherItems[otherItem2.Key])
		})
	})

	t.Run("one instance can detect if another instance has initialized the store", func(t *testing.T) {
		s.clearData("")
		s.withDefaultStore(func(store1 intf.PersistentDataStore) {
			s.withDefaultStore(func(store2 intf.PersistentDataStore) {
				assert.False(t, store1.IsInitialized())

				s.initWithEmptyData(store2)

				assert.True(t, store1.IsInitialized())
			})
		})
	})
}

func (s *PersistentDataStoreTestSuite) runGetTests(t *testing.T) {
	t.Run("existing item", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature"}
			result, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, result)

			result, err = store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})

	t.Run("nonexisting item", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			result, err := store.Get(MockData, "no")
			assert.NoError(t, err)
			assert.Nil(t, result)
		})
	})

	t.Run("all items", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			result, err := store.GetAll(MockData)
			assert.NoError(t, err)
			assert.Len(t, result, 0)

			item1 := &MockDataItem{Key: "first", Version: 1}
			item2 := &MockDataItem{Key: "second", Version: 1}
			otherItem1 := &MockDataItem{Key: "first", Version: 1, IsOtherKind: true}
			_, err = store.Upsert(MockData, item1)
			assert.NoError(t, err)
			_, err = store.Upsert(MockData, item2)
			assert.NoError(t, err)
			_, err = store.Upsert(MockOtherData, otherItem1)
			assert.NoError(t, err)

			result, err = store.GetAll(MockData)
			assert.NoError(t, err)
			assert.Len(t, result, 2)
			assert.Equal(t, item1, result[item1.Key])
			assert.Equal(t, item2, result[item2.Key])
		})
	})
}

func (s *PersistentDataStoreTestSuite) runUpsertTests(t *testing.T) {
	t.Run("newer version", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature", Version: 10, Name: "original"}
			updated, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			item1a := &MockDataItem{Key: "feature", Version: item1.Version + 1, Name: "updated"}
			updated, err = store.Upsert(MockData, item1a)
			assert.NoError(t, err)
			assert.Equal(t, item1a, updated)

			result, err := store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, item1a, result)
		})
	})

	t.Run("older version", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature", Version: 10, Name: "original"}
			updated, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			item1a := &MockDataItem{Key: "feature", Version: item1.Version - 1, Name: "updated"}
			updated, err = store.Upsert(MockData, item1a)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			result, err := store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})

	t.Run("same version", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature", Version: 10, Name: "updated"}
			updated, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			item1a := &MockDataItem{Key: "feature", Version: item1.Version, Name: "updated"}
			updated, err = store.Upsert(MockData, item1a)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			result, err := store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})
}

func (s *PersistentDataStoreTestSuite) runDeleteTests(t *testing.T) {
	t.Run("newer version", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			deletedItem := &MockDataItem{Key: item1.Key, Version: item1.Version + 1, Deleted: true}
			updated, err = store.Upsert(MockData, deletedItem)
			assert.NoError(t, err)
			assert.Equal(t, deletedItem, updated)

			result, err := store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, deletedItem, result)
		})
	})

	t.Run("older version", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			deletedItem := &MockDataItem{Key: item1.Key, Version: item1.Version - 1, Deleted: true}
			updated, err = store.Upsert(MockData, deletedItem)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			result, err := store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})

	t.Run("same version", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			deletedItem := &MockDataItem{Key: item1.Key, Version: item1.Version, Deleted: true}
			updated, err = store.Upsert(MockData, deletedItem)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			result, err := store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})

	t.Run("unknown item", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			deletedItem := &MockDataItem{Key: "feature", Version: 1, Deleted: true}
			updated, err := store.Upsert(MockData, deletedItem)
			assert.NoError(t, err)
			assert.Equal(t, deletedItem, updated)

			result, err := store.Get(MockData, deletedItem.Key)
			assert.NoError(t, err)
			assert.Equal(t, deletedItem, result)
		})
	})

	t.Run("upsert older version after delete", func(t *testing.T) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := &MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, item1, updated)

			deletedItem := &MockDataItem{Key: item1.Key, Version: item1.Version + 1, Deleted: true}
			updated, err = store.Upsert(MockData, deletedItem)
			assert.NoError(t, err)
			assert.Equal(t, deletedItem, updated)

			updated, err = store.Upsert(MockData, item1)
			assert.NoError(t, err)
			assert.Equal(t, deletedItem, updated)

			result, err := store.Get(MockData, item1.Key)
			assert.NoError(t, err)
			assert.Equal(t, deletedItem, result)
		})
	})
}

func (s *PersistentDataStoreTestSuite) runPrefixIndependenceTests(t *testing.T) {
	runWithPrefixes := func(
		t *testing.T,
		name string,
		test func(*testing.T, intf.PersistentDataStore, intf.PersistentDataStore),
	) {
		prefix1 := "testprefix1"
		prefix2 := "testprefix2"
		s.clearData(prefix1)
		s.clearData(prefix2)
		store1 := s.makeStore(prefix1)
		defer store1.Close()
		store2 := s.makeStore(prefix2)
		defer store2.Close()
		t.Run(name, func(t *testing.T) {
			test(t, store1, store2)
		})
	}

	runWithPrefixes(t, "Init", func(t *testing.T, store1 intf.PersistentDataStore, store2 intf.PersistentDataStore) {
		assert.False(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		item1a := &MockDataItem{Key: "flag-a", Version: 1}
		item1b := &MockDataItem{Key: "flag-b", Version: 1}
		item2a := &MockDataItem{Key: "flag-a", Version: 2}
		item2c := &MockDataItem{Key: "flag-c", Version: 2}

		data1 := MakeMockDataSet(item1a, item1b)
		data2 := MakeMockDataSet(item2a, item2c)

		err := store1.Init(data1)
		require.NoError(t, err)

		assert.True(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		err = store2.Init(data2)
		require.NoError(t, err)

		assert.True(t, store1.IsInitialized())
		assert.True(t, store2.IsInitialized())

		newItems1, err := store1.GetAll(MockData)
		require.NoError(t, err)
		assert.Len(t, newItems1, 2)
		assert.Equal(t, item1a, newItems1[item1a.Key])
		assert.Equal(t, item1b, newItems1[item1b.Key])

		newItem1a, err := store1.Get(MockData, item1a.Key)
		require.NoError(t, err)
		assert.Equal(t, item1a, newItem1a)

		newItem1b, err := store1.Get(MockData, item1b.Key)
		require.NoError(t, err)
		assert.Equal(t, item1b, newItem1b)

		newItems2, err := store2.GetAll(MockData)
		require.NoError(t, err)
		assert.Len(t, newItems2, 2)
		assert.Equal(t, item2a, newItems2[item2a.Key])
		assert.Equal(t, item2c, newItems2[item2c.Key])

		newItem2a, err := store2.Get(MockData, item2a.Key)
		require.NoError(t, err)
		assert.Equal(t, item2a, newItem2a)

		newItem2c, err := store2.Get(MockData, item2c.Key)
		require.NoError(t, err)
		assert.Equal(t, item2c, newItem2c)
	})

	runWithPrefixes(t, "Upsert", func(t *testing.T, store1 intf.PersistentDataStore, store2 intf.PersistentDataStore) {
		assert.False(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		key := "flag"
		item1 := &MockDataItem{Key: key, Version: 1}
		item2 := &MockDataItem{Key: key, Version: 2}

		// Start out with store1 only containing item1, and store2 only containing item2.
		// Insert the one with the higher version first, so we can verify that the version-checking logic
		// is definitely looking in the right namespace.
		updated, err := store2.Upsert(MockData, item2)
		require.NoError(t, err)
		assert.Equal(t, item2, updated)
		updated, err = store1.Upsert(MockData, item1)
		require.NoError(t, err)
		assert.Equal(t, item1, updated)

		newItem1, err := store1.Get(MockData, key)
		require.NoError(t, err)
		assert.Equal(t, item1, newItem1)

		newItem2, err := store2.Get(MockData, key)
		require.NoError(t, err)
		assert.Equal(t, item2, newItem2)

		// Now, overwrite item1 with item2 in store1.
		updated, err = store1.Upsert(MockData, item2)
		require.NoError(t, err)
		assert.Equal(t, item2, updated)

		newItem1a, err := store1.Get(MockData, key)
		require.NoError(t, err)
		assert.Equal(t, item2, newItem1a)
	})
}

func (s *PersistentDataStoreTestSuite) runConcurrentModificationTests(t *testing.T) {
	if s.concurrentModificationHookFn == nil {
		t.Skip("not implemented for this store type")
		return
	}

	s.clearData("")
	store1 := s.makeStore("")
	defer store1.Close()
	store2 := s.makeStore("")
	defer store2.Close()

	key := "foo"

	makeItemWithVersion := func(version int) *MockDataItem {
		return &MockDataItem{Key: key, Version: version}
	}

	setupStore1 := func(initialVersion int) {
		allData := MakeMockDataSet(makeItemWithVersion(initialVersion))
		require.NoError(t, store1.Init(allData))
	}

	setupConcurrentModifierToWriteVersions := func(versionsToWrite ...int) {
		i := 0
		s.concurrentModificationHookFn(store1, func() {
			if i < len(versionsToWrite) {
				newItem := makeItemWithVersion(versionsToWrite[i])
				_, err := store2.Upsert(MockData, newItem)
				require.NoError(t, err)
				i++
			}
		})
	}

	// t.Run("upsert race condition against external client with lower version", func(t *testing.T) {
	// 	setupStore1(1)
	// 	setupConcurrentModifierToWriteVersions(2, 3, 4)

	// 	_, err := store1.Upsert(MockData, key, makeItemWithVersion(10).ToSerializedItemDescriptor())
	// 	assert.NoError(t, err)

	// 	var result intf.StoreSerializedItemDescriptor
	// 	result, err = store1.Get(MockData, key)
	// 	assert.NoError(t, err)
	// 	assertEqualsSerializedItem(t, makeItemWithVersion(10), result)
	// })

	t.Run("upsert race condition against external client with higher version", func(t *testing.T) {
		setupStore1(1)
		setupConcurrentModifierToWriteVersions(3)

		_, err := store1.Upsert(MockData, makeItemWithVersion(2))
		assert.NoError(t, err)

		var result intf.VersionedData
		result, err = store1.Get(MockData, key)
		assert.NoError(t, err)
		assert.Equal(t, makeItemWithVersion(3), result)
	})
}

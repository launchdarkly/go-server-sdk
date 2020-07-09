package storetest

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	ld "gopkg.in/launchdarkly/go-server-sdk.v5"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	st "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	sh "gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/testhelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-test-helpers/v2/testbox"
)

func assertEqualsSerializedItem(
	t assert.TestingT,
	item sh.MockDataItem,
	serializedItemDesc st.SerializedItemDescriptor,
) {
	// This allows for the fact that a PersistentDataStore may not be able to get the item version without
	// deserializing it, so we allow the version to be zero.
	assert.Equal(t, item.ToSerializedItemDescriptor().SerializedItem, serializedItemDesc.SerializedItem)
	if serializedItemDesc.Version != 0 {
		assert.Equal(t, item.Version, serializedItemDesc.Version)
	}
}

func assertEqualsDeletedItem(
	t assert.TestingT,
	expected st.SerializedItemDescriptor,
	actual st.SerializedItemDescriptor,
) {
	// As above, the PersistentDataStore may not have separate access to the version and deleted state;
	// PersistentDataStoreWrapper compensates for this when it deserializes the item.
	if actual.SerializedItem == nil {
		assert.True(t, actual.Deleted)
		assert.Equal(t, expected.Version, actual.Version)
	} else {
		itemDesc, err := sh.MockData.Deserialize(actual.SerializedItem)
		assert.NoError(t, err)
		assert.Equal(t, st.ItemDescriptor{Version: expected.Version}, itemDesc)
	}
}

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
	errorStoreFactory            intf.PersistentDataStoreFactory
	errorValidator               func(assert.TestingT, error)
	concurrentModificationHookFn func(store intf.PersistentDataStore, hook func())
	includeBaseTests             bool
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
		storeFactoryFn:   storeFactoryFn,
		clearDataFn:      clearDataFn,
		includeBaseTests: true,
	}
}

// ErrorStoreFactory enables a test of error handling. The provided errorStoreFactory is expected to
// produce a data store instance whose operations should all fail and return an error. The errorValidator
// function, if any, will be called to verify that it is the expected error.
func (s *PersistentDataStoreTestSuite) ErrorStoreFactory(
	errorStoreFactory intf.PersistentDataStoreFactory,
	errorValidator func(assert.TestingT, error),
) *PersistentDataStoreTestSuite {
	s.errorStoreFactory = errorStoreFactory
	s.errorValidator = errorValidator
	return s
}

// ConcurrentModificationHook enables tests of concurrent modification behavior, for store
// implementations that support testing this.
//
// The hook parameter is a function which, when called with a store instance and another function as
// parameters, will modify the store instance so that it will call the latter function synchronously
// during each Upsert operation - after the old value has been read, but before the new one has been
// written.
func (s *PersistentDataStoreTestSuite) ConcurrentModificationHook(
	setHookFn func(store intf.PersistentDataStore, hook func()),
) *PersistentDataStoreTestSuite {
	s.concurrentModificationHookFn = setHookFn
	return s
}

// Run runs the configured test suite.
func (s *PersistentDataStoreTestSuite) Run(t *testing.T) {
	s.runInternal(testbox.RealTest(t))
}

func (s *PersistentDataStoreTestSuite) runInternal(t testbox.TestingT) {
	if s.includeBaseTests { // PersistentDataStoreTestSuiteTest can disable these
		t.Run("Init", s.runInitTests)
		t.Run("Get", s.runGetTests)
		t.Run("Upsert", s.runUpsertTests)
		t.Run("Delete", s.runDeleteTests)

		t.Run("IsStoreAvailable", func(t testbox.TestingT) {
			// The store should always be available during this test suite
			s.withDefaultStore(func(store intf.PersistentDataStore) {
				assert.True(t, store.IsStoreAvailable())
			})
		})
	}

	t.Run("error returns", s.runErrorTests)
	t.Run("prefix independence", s.runPrefixIndependenceTests)
	t.Run("concurrent modification", s.runConcurrentModificationTests)

	if s.includeBaseTests {
		t.Run("LDClient end-to-end tests", s.runLDClientEndToEndTests)
	}
}

func (s *PersistentDataStoreTestSuite) makeStore(prefix string) intf.PersistentDataStore {
	store, err := s.storeFactoryFn(prefix).CreatePersistentDataStore(testhelpers.NewSimpleClientContext(""))
	if err != nil {
		panic(err) // COVERAGE: can't cause this condition in PersistentDataStoreTestSuiteTest
	}
	return store
}

func (s *PersistentDataStoreTestSuite) clearData(prefix string) {
	err := s.clearDataFn(prefix)
	if err != nil {
		panic(err) // COVERAGE: can't cause this condition in PersistentDataStoreTestSuiteTest
	}
}

func (s *PersistentDataStoreTestSuite) initWithEmptyData(store intf.PersistentDataStore) {
	_ = store.Init(sh.MakeSerializedMockDataSet())
	// We are ignoring the error here because the store might have been configured to deliberately
	// cause an error, for tests that validate error handling.
}

func (s *PersistentDataStoreTestSuite) withDefaultStore(action func(intf.PersistentDataStore)) {
	store := s.makeStore("")
	defer store.Close() //nolint:errcheck
	action(store)
}

func (s *PersistentDataStoreTestSuite) withDefaultInitedStore(action func(intf.PersistentDataStore)) {
	s.clearData("")
	store := s.makeStore("")
	defer store.Close() //nolint:errcheck
	s.initWithEmptyData(store)
	action(store)
}

func (s *PersistentDataStoreTestSuite) runInitTests(t testbox.TestingT) {
	t.Run("store initialized after init", func(t testbox.TestingT) {
		s.clearData("")
		s.withDefaultStore(func(store intf.PersistentDataStore) {
			item1 := sh.MockDataItem{Key: "feature"}
			allData := sh.MakeSerializedMockDataSet(item1)
			require.NoError(t, store.Init(allData))

			assert.True(t, store.IsInitialized())
		})
	})

	t.Run("completely replaces previous data", func(t testbox.TestingT) {
		s.clearData("")
		s.withDefaultStore(func(store intf.PersistentDataStore) {
			item1 := sh.MockDataItem{Key: "first", Version: 1}
			item2 := sh.MockDataItem{Key: "second", Version: 1}
			otherItem1 := sh.MockDataItem{Key: "first", Version: 1, IsOtherKind: true}
			allData := sh.MakeSerializedMockDataSet(item1, item2, otherItem1)
			require.NoError(t, store.Init(allData))

			items, err := store.GetAll(sh.MockData)
			require.NoError(t, err)
			assert.Len(t, items, 2)
			assertEqualsSerializedItem(t, item1, itemDescriptorsToMap(items)[item1.Key])
			assertEqualsSerializedItem(t, item2, itemDescriptorsToMap(items)[item2.Key])

			otherItems, err := store.GetAll(sh.MockOtherData)
			require.NoError(t, err)
			assert.Len(t, otherItems, 1)
			assertEqualsSerializedItem(t, otherItem1, itemDescriptorsToMap(otherItems)[otherItem1.Key])

			otherItem2 := sh.MockDataItem{Key: "second", Version: 1, IsOtherKind: true}
			allData = sh.MakeSerializedMockDataSet(item1, otherItem2)
			require.NoError(t, store.Init(allData))

			items, err = store.GetAll(sh.MockData)
			require.NoError(t, err)
			assert.Len(t, items, 1)
			assertEqualsSerializedItem(t, item1, itemDescriptorsToMap(items)[item1.Key])

			otherItems, err = store.GetAll(sh.MockOtherData)
			require.NoError(t, err)
			assert.Len(t, otherItems, 1)
			assertEqualsSerializedItem(t, otherItem2, itemDescriptorsToMap(otherItems)[otherItem2.Key])
		})
	})

	t.Run("one instance can detect if another instance has initialized the store", func(t testbox.TestingT) {
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

func (s *PersistentDataStoreTestSuite) runGetTests(t testbox.TestingT) {
	t.Run("existing item", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := sh.MockDataItem{Key: "feature"}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("nonexisting item", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			result, err := store.Get(sh.MockData, "no")
			assert.NoError(t, err)
			assert.Equal(t, -1, result.Version)
			assert.Nil(t, result.SerializedItem)
		})
	})

	t.Run("all items", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			result, err := store.GetAll(sh.MockData)
			assert.NoError(t, err)
			assert.Len(t, result, 0)

			item1 := sh.MockDataItem{Key: "first", Version: 1}
			item2 := sh.MockDataItem{Key: "second", Version: 1}
			otherItem1 := sh.MockDataItem{Key: "first", Version: 1, IsOtherKind: true}
			_, err = store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			_, err = store.Upsert(sh.MockData, item2.Key, item2.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			_, err = store.Upsert(sh.MockOtherData, otherItem1.Key, otherItem1.ToSerializedItemDescriptor())
			assert.NoError(t, err)

			result, err = store.GetAll(sh.MockData)
			assert.NoError(t, err)
			assert.Len(t, result, 2)
			assertEqualsSerializedItem(t, item1, itemDescriptorsToMap(result)[item1.Key])
			assertEqualsSerializedItem(t, item2, itemDescriptorsToMap(result)[item2.Key])
		})
	})
}

func (s *PersistentDataStoreTestSuite) runUpsertTests(t testbox.TestingT) {
	item1 := sh.MockDataItem{Key: "feature", Version: 10, Name: "original"}

	setupItem1 := func(t testbox.TestingT, store intf.PersistentDataStore) {
		updated, err := store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
		assert.NoError(t, err)
		assert.True(t, updated)
	}

	t.Run("newer version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			setupItem1(t, store)

			item1a := sh.MockDataItem{Key: "feature", Version: item1.Version + 1, Name: "updated"}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1a.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1a, result)
		})
	})

	t.Run("older version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			setupItem1(t, store)

			item1a := sh.MockDataItem{Key: "feature", Version: item1.Version - 1, Name: "updated"}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1a.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("same version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			setupItem1(t, store)

			item1a := sh.MockDataItem{Key: "feature", Version: item1.Version, Name: "updated"}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1a.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})
}

func (s *PersistentDataStoreTestSuite) runDeleteTests(t testbox.TestingT) {
	t.Run("newer version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := sh.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := sh.MockDataItem{Key: item1.Key, Version: item1.Version + 1, Deleted: true}
			updated, err = store.Upsert(sh.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsDeletedItem(t, deletedItem.ToSerializedItemDescriptor(), result)
		})
	})

	t.Run("older version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := sh.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := sh.MockDataItem{Key: item1.Key, Version: item1.Version - 1, Deleted: true}
			updated, err = store.Upsert(sh.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("same version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := sh.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := sh.MockDataItem{Key: item1.Key, Version: item1.Version, Deleted: true}
			updated, err = store.Upsert(sh.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("unknown item", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			deletedItem := sh.MockDataItem{Key: "feature", Version: 1, Deleted: true}
			updated, err := store.Upsert(sh.MockData, deletedItem.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(sh.MockData, deletedItem.Key)
			assert.NoError(t, err)
			assertEqualsDeletedItem(t, deletedItem.ToSerializedItemDescriptor(), result)
		})
	})

	t.Run("upsert older version after delete", func(t testbox.TestingT) {
		s.withDefaultInitedStore(func(store intf.PersistentDataStore) {
			item1 := sh.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := sh.MockDataItem{Key: item1.Key, Version: item1.Version + 1, Deleted: true}
			updated, err = store.Upsert(sh.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			updated, err = store.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(sh.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsDeletedItem(t, deletedItem.ToSerializedItemDescriptor(), result)
		})
	})
}

func (s *PersistentDataStoreTestSuite) runPrefixIndependenceTests(t testbox.TestingT) {
	runWithPrefixes := func(
		t testbox.TestingT,
		name string,
		test func(testbox.TestingT, intf.PersistentDataStore, intf.PersistentDataStore),
	) {
		prefix1 := "testprefix1"
		prefix2 := "testprefix2"
		s.clearData(prefix1)
		s.clearData(prefix2)
		store1 := s.makeStore(prefix1)
		defer store1.Close() //nolint:errcheck
		store2 := s.makeStore(prefix2)
		defer store2.Close() //nolint:errcheck
		t.Run(name, func(t testbox.TestingT) {
			test(t, store1, store2)
		})
	}

	runWithPrefixes(t, "Init", func(t testbox.TestingT, store1 intf.PersistentDataStore, store2 intf.PersistentDataStore) {
		assert.False(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		item1a := sh.MockDataItem{Key: "flag-a", Version: 1}
		item1b := sh.MockDataItem{Key: "flag-b", Version: 1}
		item2a := sh.MockDataItem{Key: "flag-a", Version: 2}
		item2c := sh.MockDataItem{Key: "flag-c", Version: 2}

		data1 := sh.MakeSerializedMockDataSet(item1a, item1b)
		data2 := sh.MakeSerializedMockDataSet(item2a, item2c)

		err := store1.Init(data1)
		require.NoError(t, err)

		assert.True(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		err = store2.Init(data2)
		require.NoError(t, err)

		assert.True(t, store1.IsInitialized())
		assert.True(t, store2.IsInitialized())

		newItems1, err := store1.GetAll(sh.MockData)
		require.NoError(t, err)
		assert.Len(t, newItems1, 2)
		assertEqualsSerializedItem(t, item1a, itemDescriptorsToMap(newItems1)[item1a.Key])
		assertEqualsSerializedItem(t, item1b, itemDescriptorsToMap(newItems1)[item1b.Key])

		newItem1a, err := store1.Get(sh.MockData, item1a.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item1a, newItem1a)

		newItem1b, err := store1.Get(sh.MockData, item1b.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item1b, newItem1b)

		newItems2, err := store2.GetAll(sh.MockData)
		require.NoError(t, err)
		assert.Len(t, newItems2, 2)
		assertEqualsSerializedItem(t, item2a, itemDescriptorsToMap(newItems2)[item2a.Key])
		assertEqualsSerializedItem(t, item2c, itemDescriptorsToMap(newItems2)[item2c.Key])

		newItem2a, err := store2.Get(sh.MockData, item2a.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item2a, newItem2a)

		newItem2c, err := store2.Get(sh.MockData, item2c.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item2c, newItem2c)
	})

	runWithPrefixes(t, "Upsert/Delete", func(t testbox.TestingT, store1 intf.PersistentDataStore,
		store2 intf.PersistentDataStore) {
		assert.False(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		key := "flag"
		item1 := sh.MockDataItem{Key: key, Version: 1}
		item2 := sh.MockDataItem{Key: key, Version: 2}

		// Insert the one with the higher version first, so we can verify that the version-checking logic
		// is definitely looking in the right namespace
		updated, err := store2.Upsert(sh.MockData, item2.Key, item2.ToSerializedItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)
		_, err = store1.Upsert(sh.MockData, item1.Key, item1.ToSerializedItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)

		newItem1, err := store1.Get(sh.MockData, key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item1, newItem1)

		newItem2, err := store2.Get(sh.MockData, key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item2, newItem2)

		updated, err = store1.Upsert(sh.MockData, key, item2.ToSerializedItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)

		newItem1a, err := store1.Get(sh.MockData, key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item2, newItem1a)
	})
}

func (s *PersistentDataStoreTestSuite) runErrorTests(t testbox.TestingT) {
	if s.errorStoreFactory == nil {
		t.Skip("not implemented for this store type")
		return
	}
	errorValidator := s.errorValidator
	if errorValidator == nil {
		errorValidator = func(assert.TestingT, error) {}
	}

	store, err := s.errorStoreFactory.CreatePersistentDataStore(sh.NewSimpleTestContext(""))
	require.NoError(t, err)
	defer store.Close() //nolint:errcheck

	t.Run("Init", func(t testbox.TestingT) {
		allData := []st.SerializedCollection{
			{Kind: datakinds.Features},
			{Kind: datakinds.Segments},
		}
		err := store.Init(allData)
		require.Error(t, err)
		errorValidator(t, err)
	})

	t.Run("Get", func(t testbox.TestingT) {
		_, err := store.Get(datakinds.Features, "key")
		require.Error(t, err)
		errorValidator(t, err)
	})

	t.Run("GetAll", func(t testbox.TestingT) {
		_, err := store.GetAll(datakinds.Features)
		require.Error(t, err)
		errorValidator(t, err)
	})

	t.Run("Upsert", func(t testbox.TestingT) {
		desc := sh.FlagDescriptor(ldbuilders.NewFlagBuilder("key").Build())
		sdesc := st.SerializedItemDescriptor{
			Version:        1,
			SerializedItem: datakinds.Features.Serialize(desc),
		}
		_, err := store.Upsert(datakinds.Features, "key", sdesc)
		require.Error(t, err)
		errorValidator(t, err)
	})

	t.Run("IsInitialized", func(t testbox.TestingT) {
		assert.False(t, store.IsInitialized())
	})
}

func (s *PersistentDataStoreTestSuite) runConcurrentModificationTests(t testbox.TestingT) {
	if s.concurrentModificationHookFn == nil {
		t.Skip("not implemented for this store type")
		return
	}

	s.clearData("")
	store1 := s.makeStore("")
	defer store1.Close() //nolint:errcheck
	store2 := s.makeStore("")
	defer store2.Close() //nolint:errcheck

	key := "foo"

	makeItemWithVersion := func(version int) sh.MockDataItem {
		return sh.MockDataItem{Key: key, Version: version}
	}

	setupStore1 := func(initialVersion int) {
		allData := sh.MakeSerializedMockDataSet(makeItemWithVersion(initialVersion))
		require.NoError(t, store1.Init(allData))
	}

	setupConcurrentModifierToWriteVersions := func(versionsToWrite ...int) {
		i := 0
		s.concurrentModificationHookFn(store1, func() {
			if i < len(versionsToWrite) {
				newItem := makeItemWithVersion(versionsToWrite[i])
				_, err := store2.Upsert(sh.MockData, key, newItem.ToSerializedItemDescriptor())
				require.NoError(t, err)
				i++
			}
		})
	}

	t.Run("upsert race condition against external client with lower version", func(t testbox.TestingT) {
		setupStore1(1)
		setupConcurrentModifierToWriteVersions(2, 3, 4)

		_, err := store1.Upsert(sh.MockData, key, makeItemWithVersion(10).ToSerializedItemDescriptor())
		assert.NoError(t, err)

		var result st.SerializedItemDescriptor
		result, err = store1.Get(sh.MockData, key)
		assert.NoError(t, err)
		assertEqualsSerializedItem(t, makeItemWithVersion(10), result)
	})

	t.Run("upsert race condition against external client with higher version", func(t testbox.TestingT) {
		setupStore1(1)
		setupConcurrentModifierToWriteVersions(3)

		updated, err := store1.Upsert(sh.MockData, key, makeItemWithVersion(2).ToSerializedItemDescriptor())
		assert.NoError(t, err)
		assert.False(t, updated)

		var result st.SerializedItemDescriptor
		result, err = store1.Get(sh.MockData, key)
		assert.NoError(t, err)
		assertEqualsSerializedItem(t, makeItemWithVersion(3), result)
	})
}

func itemDescriptorsToMap(
	items []st.KeyedSerializedItemDescriptor,
) map[string]st.SerializedItemDescriptor {
	ret := make(map[string]st.SerializedItemDescriptor)
	for _, item := range items {
		ret[item.Key] = item.Item
	}
	return ret
}

func (s *PersistentDataStoreTestSuite) runLDClientEndToEndTests(t testbox.TestingT) {
	dataStoreFactory := s.storeFactoryFn("ldclient")

	// This is a basic smoke test to verify that the data store component behaves correctly within an
	// SDK client instance.

	flagKey, segmentKey, userKey, otherUserKey := "flagkey", "segmentkey", "userkey", "otheruser"
	goodValue1, goodValue2, badValue := ldvalue.String("good"), ldvalue.String("better"), ldvalue.String("bad")
	goodVariation1, goodVariation2, badVariation := 0, 1, 2
	user, otherUser := lduser.NewUser(userKey), lduser.NewUser(otherUserKey)

	makeFlagThatReturnsVariationForSegmentMatch := func(version int, variation int) ldmodel.FeatureFlag {
		return ldbuilders.NewFlagBuilder(flagKey).Version(version).
			On(true).
			Variations(goodValue1, goodValue2, badValue).
			FallthroughVariation(badVariation).
			AddRule(ldbuilders.NewRuleBuilder().Variation(variation).Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segmentKey)),
			)).
			Build()
	}
	makeSegmentThatMatchesUserKeys := func(version int, keys ...string) ldmodel.Segment {
		return ldbuilders.NewSegmentBuilder(segmentKey).Version(version).
			Included(keys...).
			Build()
	}
	flag := makeFlagThatReturnsVariationForSegmentMatch(1, goodVariation1)
	segment := makeSegmentThatMatchesUserKeys(1, userKey)

	data := []st.Collection{
		{Kind: datakinds.Features, Items: []st.KeyedItemDescriptor{
			{Key: flagKey, Item: sh.FlagDescriptor(flag)},
		}},
		{Kind: datakinds.Segments, Items: []st.KeyedItemDescriptor{
			{Key: segmentKey, Item: sh.SegmentDescriptor(segment)},
		}},
	}
	dataSourceFactory := &sh.DataSourceFactoryThatExposesUpdater{ // allows us to simulate an update
		UnderlyingFactory: &sh.DataSourceFactoryWithData{Data: data},
	}
	mockLog := ldlogtest.NewMockLog()
	config := ld.Config{
		DataStore:  ldcomponents.PersistentDataStore(dataStoreFactory).NoCaching(),
		DataSource: dataSourceFactory,
		Events:     ldcomponents.NoEvents(),
		Logging:    ldcomponents.Logging().Loggers(mockLog.Loggers),
	}

	client, err := ld.MakeCustomClient("sdk-key", config, 5*time.Second)
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck

	flagShouldHaveValueForUser := func(u lduser.User, expectedValue ldvalue.Value) {
		value, err := client.JSONVariation(flagKey, u, ldvalue.Null())
		assert.NoError(t, err)
		assert.Equal(t, expectedValue, value)
	}

	t.Run("get flag", func(t testbox.TestingT) {
		flagShouldHaveValueForUser(user, goodValue1)
		flagShouldHaveValueForUser(otherUser, badValue)
	})

	t.Run("get all flags", func(t testbox.TestingT) {
		state := client.AllFlagsState(user)
		assert.Equal(t, map[string]ldvalue.Value{flagKey: goodValue1}, state.ToValuesMap())
	})

	t.Run("update flag", func(t testbox.TestingT) {
		flagv2 := makeFlagThatReturnsVariationForSegmentMatch(2, goodVariation2)
		dataSourceFactory.DataSourceUpdates.Upsert(datakinds.Features, flagKey,
			sh.FlagDescriptor(flagv2))

		flagShouldHaveValueForUser(user, goodValue2)
		flagShouldHaveValueForUser(otherUser, badValue)
	})

	t.Run("update segment", func(t testbox.TestingT) {
		segmentv2 := makeSegmentThatMatchesUserKeys(2, userKey, otherUserKey)
		dataSourceFactory.DataSourceUpdates.Upsert(datakinds.Segments, segmentKey,
			sh.SegmentDescriptor(segmentv2))
		flagShouldHaveValueForUser(otherUser, goodValue2) // otherUser is now matched by the segment
	})

	t.Run("delete segment", func(t testbox.TestingT) {
		// deleting the segment should cause the flag that uses it to stop matching
		dataSourceFactory.DataSourceUpdates.Upsert(datakinds.Segments, segmentKey,
			st.ItemDescriptor{Version: 3, Item: nil})
		flagShouldHaveValueForUser(user, badValue)
	})

	t.Run("delete flag", func(t testbox.TestingT) {
		// deleting the flag should cause the flag to become unknown
		dataSourceFactory.DataSourceUpdates.Upsert(datakinds.Features, flagKey,
			st.ItemDescriptor{Version: 3, Item: nil})
		value, detail, err := client.JSONVariationDetail(flagKey, user, ldvalue.Null())
		assert.Error(t, err)
		assert.Equal(t, ldvalue.Null(), value)
		assert.Equal(t, ldreason.EvalErrorFlagNotFound, detail.Reason.GetErrorKind())
	})

	t.Run("no errors are logged", func(t testbox.TestingT) {
		assert.Len(t, mockLog.GetOutput(ldlog.Error), 0)
	})
}

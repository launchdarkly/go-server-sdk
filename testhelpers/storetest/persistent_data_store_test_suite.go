package storetest

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"

	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"
	ld "github.com/launchdarkly/go-server-sdk/v6"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datakinds"
	sh "github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/ldcomponents"
	ssys "github.com/launchdarkly/go-server-sdk/v6/subsystems"
	st "github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"
	"github.com/launchdarkly/go-server-sdk/v6/testhelpers"

	"github.com/launchdarkly/go-test-helpers/v3/testbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertEqualsSerializedItem(
	t assert.TestingT,
	item mocks.MockDataItem,
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
		itemDesc, err := mocks.MockData.Deserialize(actual.SerializedItem)
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
	storeFactoryFn               func(string) ssys.ComponentConfigurer[ssys.PersistentDataStore]
	clearDataFn                  func(string) error
	errorStoreFactory            ssys.ComponentConfigurer[ssys.PersistentDataStore]
	errorValidator               func(assert.TestingT, error)
	concurrentModificationHookFn func(store ssys.PersistentDataStore, hook func())
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
	storeFactoryFn func(prefix string) ssys.ComponentConfigurer[ssys.PersistentDataStore],
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
	errorStoreFactory ssys.ComponentConfigurer[ssys.PersistentDataStore],
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
	setHookFn func(store ssys.PersistentDataStore, hook func()),
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
			s.withDefaultStore(t, func(store ssys.PersistentDataStore) {
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

func (s *PersistentDataStoreTestSuite) clearData(t require.TestingT, prefix string) {
	require.NoError(t, s.clearDataFn(prefix))
}

func (s *PersistentDataStoreTestSuite) initWithEmptyData(store ssys.PersistentDataStore) {
	_ = store.Init(mocks.MakeSerializedMockDataSet())
	// We are ignoring the error here because the store might have been configured to deliberately
	// cause an error, for tests that validate error handling.
}

func (s *PersistentDataStoreTestSuite) withStore(
	t testbox.TestingT,
	prefix string,
	action func(ssys.PersistentDataStore),
) {
	testhelpers.WithMockLoggingContext(t, func(context ssys.ClientContext) {
		store, err := s.storeFactoryFn(prefix).Build(context)
		require.NoError(t, err)
		defer func() {
			_ = store.Close()
		}()
		action(store)
	})
}

func (s *PersistentDataStoreTestSuite) withDefaultStore(
	t testbox.TestingT,
	action func(ssys.PersistentDataStore),
) {
	s.withStore(t, "", action)
}

func (s *PersistentDataStoreTestSuite) withDefaultInitedStore(
	t testbox.TestingT,
	action func(ssys.PersistentDataStore),
) {
	s.clearData(t, "")
	s.withDefaultStore(t, func(store ssys.PersistentDataStore) {
		s.initWithEmptyData(store)
		action(store)
	})
}

func (s *PersistentDataStoreTestSuite) runInitTests(t testbox.TestingT) {
	t.Run("store initialized after init", func(t testbox.TestingT) {
		s.clearData(t, "")
		s.withDefaultStore(t, func(store ssys.PersistentDataStore) {
			item1 := mocks.MockDataItem{Key: "feature"}
			allData := mocks.MakeSerializedMockDataSet(item1)
			require.NoError(t, store.Init(allData))

			assert.True(t, store.IsInitialized())
		})
	})

	t.Run("completely replaces previous data", func(t testbox.TestingT) {
		s.clearData(t, "")
		s.withDefaultStore(t, func(store ssys.PersistentDataStore) {
			item1 := mocks.MockDataItem{Key: "first", Version: 1}
			item2 := mocks.MockDataItem{Key: "second", Version: 1}
			otherItem1 := mocks.MockDataItem{Key: "first", Version: 1, IsOtherKind: true}
			allData := mocks.MakeSerializedMockDataSet(item1, item2, otherItem1)
			require.NoError(t, store.Init(allData))

			items, err := store.GetAll(mocks.MockData)
			require.NoError(t, err)
			assert.Len(t, items, 2)
			assertEqualsSerializedItem(t, item1, itemDescriptorsToMap(items)[item1.Key])
			assertEqualsSerializedItem(t, item2, itemDescriptorsToMap(items)[item2.Key])

			otherItems, err := store.GetAll(mocks.MockOtherData)
			require.NoError(t, err)
			assert.Len(t, otherItems, 1)
			assertEqualsSerializedItem(t, otherItem1, itemDescriptorsToMap(otherItems)[otherItem1.Key])

			otherItem2 := mocks.MockDataItem{Key: "second", Version: 1, IsOtherKind: true}
			allData = mocks.MakeSerializedMockDataSet(item1, otherItem2)
			require.NoError(t, store.Init(allData))

			items, err = store.GetAll(mocks.MockData)
			require.NoError(t, err)
			assert.Len(t, items, 1)
			assertEqualsSerializedItem(t, item1, itemDescriptorsToMap(items)[item1.Key])

			otherItems, err = store.GetAll(mocks.MockOtherData)
			require.NoError(t, err)
			assert.Len(t, otherItems, 1)
			assertEqualsSerializedItem(t, otherItem2, itemDescriptorsToMap(otherItems)[otherItem2.Key])
		})
	})

	t.Run("one instance can detect if another instance has initialized the store", func(t testbox.TestingT) {
		s.clearData(t, "")
		s.withDefaultStore(t, func(store1 ssys.PersistentDataStore) {
			s.withDefaultStore(t, func(store2 ssys.PersistentDataStore) {
				assert.False(t, store1.IsInitialized())

				s.initWithEmptyData(store2)

				assert.True(t, store1.IsInitialized())
			})
		})
	})
}

func (s *PersistentDataStoreTestSuite) runGetTests(t testbox.TestingT) {
	t.Run("existing item", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			item1 := mocks.MockDataItem{Key: "feature"}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("nonexisting item", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			result, err := store.Get(mocks.MockData, "no")
			assert.NoError(t, err)
			assert.Equal(t, -1, result.Version)
			assert.Nil(t, result.SerializedItem)
		})
	})

	t.Run("all items", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			result, err := store.GetAll(mocks.MockData)
			assert.NoError(t, err)
			assert.Len(t, result, 0)

			item1 := mocks.MockDataItem{Key: "first", Version: 1}
			item2 := mocks.MockDataItem{Key: "second", Version: 1}
			otherItem1 := mocks.MockDataItem{Key: "first", Version: 1, IsOtherKind: true}
			_, err = store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			_, err = store.Upsert(mocks.MockData, item2.Key, item2.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			_, err = store.Upsert(mocks.MockOtherData, otherItem1.Key, otherItem1.ToSerializedItemDescriptor())
			assert.NoError(t, err)

			result, err = store.GetAll(mocks.MockData)
			assert.NoError(t, err)
			assert.Len(t, result, 2)
			assertEqualsSerializedItem(t, item1, itemDescriptorsToMap(result)[item1.Key])
			assertEqualsSerializedItem(t, item2, itemDescriptorsToMap(result)[item2.Key])
		})
	})
}

func (s *PersistentDataStoreTestSuite) runUpsertTests(t testbox.TestingT) {
	item1 := mocks.MockDataItem{Key: "feature", Version: 10, Name: "original"}

	setupItem1 := func(t testbox.TestingT, store ssys.PersistentDataStore) {
		updated, err := store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
		assert.NoError(t, err)
		assert.True(t, updated)
	}

	t.Run("newer version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			setupItem1(t, store)

			item1a := mocks.MockDataItem{Key: "feature", Version: item1.Version + 1, Name: "updated"}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1a.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1a, result)
		})
	})

	t.Run("older version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			setupItem1(t, store)

			item1a := mocks.MockDataItem{Key: "feature", Version: item1.Version - 1, Name: "updated"}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1a.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("same version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			setupItem1(t, store)

			item1a := mocks.MockDataItem{Key: "feature", Version: item1.Version, Name: "updated"}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1a.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})
}

func (s *PersistentDataStoreTestSuite) runDeleteTests(t testbox.TestingT) {
	t.Run("newer version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			item1 := mocks.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := mocks.MockDataItem{Key: item1.Key, Version: item1.Version + 1, Deleted: true}
			updated, err = store.Upsert(mocks.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsDeletedItem(t, deletedItem.ToSerializedItemDescriptor(), result)
		})
	})

	t.Run("older version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			item1 := mocks.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := mocks.MockDataItem{Key: item1.Key, Version: item1.Version - 1, Deleted: true}
			updated, err = store.Upsert(mocks.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("same version", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			item1 := mocks.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := mocks.MockDataItem{Key: item1.Key, Version: item1.Version, Deleted: true}
			updated, err = store.Upsert(mocks.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsSerializedItem(t, item1, result)
		})
	})

	t.Run("unknown item", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			deletedItem := mocks.MockDataItem{Key: "feature", Version: 1, Deleted: true}
			updated, err := store.Upsert(mocks.MockData, deletedItem.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(mocks.MockData, deletedItem.Key)
			assert.NoError(t, err)
			assertEqualsDeletedItem(t, deletedItem.ToSerializedItemDescriptor(), result)
		})
	})

	t.Run("upsert older version after delete", func(t testbox.TestingT) {
		s.withDefaultInitedStore(t, func(store ssys.PersistentDataStore) {
			item1 := mocks.MockDataItem{Key: "feature", Version: 10}
			updated, err := store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			deletedItem := mocks.MockDataItem{Key: item1.Key, Version: item1.Version + 1, Deleted: true}
			updated, err = store.Upsert(mocks.MockData, item1.Key, deletedItem.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.True(t, updated)

			updated, err = store.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
			assert.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(mocks.MockData, item1.Key)
			assert.NoError(t, err)
			assertEqualsDeletedItem(t, deletedItem.ToSerializedItemDescriptor(), result)
		})
	})
}

func (s *PersistentDataStoreTestSuite) runPrefixIndependenceTests(t testbox.TestingT) {
	runWithPrefixes := func(
		t testbox.TestingT,
		name string,
		test func(testbox.TestingT, ssys.PersistentDataStore, ssys.PersistentDataStore),
	) {
		prefix1 := "testprefix1"
		prefix2 := "testprefix2"
		s.clearData(t, prefix1)
		s.clearData(t, prefix2)

		s.withStore(t, prefix1, func(store1 ssys.PersistentDataStore) {
			s.withStore(t, prefix2, func(store2 ssys.PersistentDataStore) {
				t.Run(name, func(t testbox.TestingT) {
					test(t, store1, store2)
				})
			})
		})
	}

	runWithPrefixes(t, "Init", func(t testbox.TestingT, store1 ssys.PersistentDataStore, store2 ssys.PersistentDataStore) {
		assert.False(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		item1a := mocks.MockDataItem{Key: "flag-a", Version: 1}
		item1b := mocks.MockDataItem{Key: "flag-b", Version: 1}
		item2a := mocks.MockDataItem{Key: "flag-a", Version: 2}
		item2c := mocks.MockDataItem{Key: "flag-c", Version: 2}

		data1 := mocks.MakeSerializedMockDataSet(item1a, item1b)
		data2 := mocks.MakeSerializedMockDataSet(item2a, item2c)

		err := store1.Init(data1)
		require.NoError(t, err)

		assert.True(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		err = store2.Init(data2)
		require.NoError(t, err)

		assert.True(t, store1.IsInitialized())
		assert.True(t, store2.IsInitialized())

		newItems1, err := store1.GetAll(mocks.MockData)
		require.NoError(t, err)
		assert.Len(t, newItems1, 2)
		assertEqualsSerializedItem(t, item1a, itemDescriptorsToMap(newItems1)[item1a.Key])
		assertEqualsSerializedItem(t, item1b, itemDescriptorsToMap(newItems1)[item1b.Key])

		newItem1a, err := store1.Get(mocks.MockData, item1a.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item1a, newItem1a)

		newItem1b, err := store1.Get(mocks.MockData, item1b.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item1b, newItem1b)

		newItems2, err := store2.GetAll(mocks.MockData)
		require.NoError(t, err)
		assert.Len(t, newItems2, 2)
		assertEqualsSerializedItem(t, item2a, itemDescriptorsToMap(newItems2)[item2a.Key])
		assertEqualsSerializedItem(t, item2c, itemDescriptorsToMap(newItems2)[item2c.Key])

		newItem2a, err := store2.Get(mocks.MockData, item2a.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item2a, newItem2a)

		newItem2c, err := store2.Get(mocks.MockData, item2c.Key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item2c, newItem2c)
	})

	runWithPrefixes(t, "Upsert/Delete", func(t testbox.TestingT, store1 ssys.PersistentDataStore,
		store2 ssys.PersistentDataStore) {
		assert.False(t, store1.IsInitialized())
		assert.False(t, store2.IsInitialized())

		key := "flag"
		item1 := mocks.MockDataItem{Key: key, Version: 1}
		item2 := mocks.MockDataItem{Key: key, Version: 2}

		// Insert the one with the higher version first, so we can verify that the version-checking logic
		// is definitely looking in the right namespace
		updated, err := store2.Upsert(mocks.MockData, item2.Key, item2.ToSerializedItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)
		_, err = store1.Upsert(mocks.MockData, item1.Key, item1.ToSerializedItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)

		newItem1, err := store1.Get(mocks.MockData, key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item1, newItem1)

		newItem2, err := store2.Get(mocks.MockData, key)
		require.NoError(t, err)
		assertEqualsSerializedItem(t, item2, newItem2)

		updated, err = store1.Upsert(mocks.MockData, key, item2.ToSerializedItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)

		newItem1a, err := store1.Get(mocks.MockData, key)
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

	store, err := s.errorStoreFactory.Build(sh.NewSimpleTestContext(""))
	require.NoError(t, err)
	defer store.Close() //nolint:errcheck

	t.Run("Init", func(t testbox.TestingT) {
		allData := []st.SerializedCollection{
			{Kind: datakinds.Features},
			{Kind: datakinds.Segments},
			{Kind: datakinds.ConfigOverrides},
			{Kind: datakinds.Metrics},
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

	key := "foo"

	makeItemWithVersion := func(version int) mocks.MockDataItem {
		return mocks.MockDataItem{Key: key, Version: version}
	}

	s.clearData(t, "")
	s.withStore(t, "", func(store1 ssys.PersistentDataStore) {
		s.withStore(t, "", func(store2 ssys.PersistentDataStore) {
			setupStore1 := func(initialVersion int) {
				allData := mocks.MakeSerializedMockDataSet(makeItemWithVersion(initialVersion))
				require.NoError(t, store1.Init(allData))
			}

			setupConcurrentModifierToWriteVersions := func(versionsToWrite ...int) {
				i := 0
				s.concurrentModificationHookFn(store1, func() {
					if i < len(versionsToWrite) {
						newItem := makeItemWithVersion(versionsToWrite[i])
						_, err := store2.Upsert(mocks.MockData, key, newItem.ToSerializedItemDescriptor())
						require.NoError(t, err)
						i++
					}
				})
			}

			t.Run("upsert race condition against external client with lower version", func(t testbox.TestingT) {
				setupStore1(1)
				setupConcurrentModifierToWriteVersions(2, 3, 4)

				_, err := store1.Upsert(mocks.MockData, key, makeItemWithVersion(10).ToSerializedItemDescriptor())
				assert.NoError(t, err)

				var result st.SerializedItemDescriptor
				result, err = store1.Get(mocks.MockData, key)
				assert.NoError(t, err)
				assertEqualsSerializedItem(t, makeItemWithVersion(10), result)
			})

			t.Run("upsert race condition against external client with higher version", func(t testbox.TestingT) {
				setupStore1(1)
				setupConcurrentModifierToWriteVersions(3)

				updated, err := store1.Upsert(mocks.MockData, key, makeItemWithVersion(2).ToSerializedItemDescriptor())
				assert.NoError(t, err)
				assert.False(t, updated)

				var result st.SerializedItemDescriptor
				result, err = store1.Get(mocks.MockData, key)
				assert.NoError(t, err)
				assertEqualsSerializedItem(t, makeItemWithVersion(3), result)
			})
		})
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

	flagKey, segmentKey, configOverrideKey, metricKey, userKey, otherUserKey :=
		"flagkey", "segmentkey", "overridekey", "metrickey", "userkey", "otheruser"
	goodValue1, goodValue2, badValue := ldvalue.String("good"), ldvalue.String("better"), ldvalue.String("bad")
	goodVariation1, goodVariation2, badVariation := 0, 1, 2
	user, otherUser := ldcontext.New(userKey), ldcontext.New(otherUserKey)

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
	override := ldbuilders.NewConfigOverrideBuilder(configOverrideKey).Build()
	metric := ldbuilders.NewMetricBuilder(metricKey).Build()

	data := []st.Collection{
		{Kind: datakinds.Features, Items: []st.KeyedItemDescriptor{
			{Key: flagKey, Item: sh.FlagDescriptor(flag)},
		}},
		{Kind: datakinds.Segments, Items: []st.KeyedItemDescriptor{
			{Key: segmentKey, Item: sh.SegmentDescriptor(segment)},
		}},
		{Kind: datakinds.ConfigOverrides, Items: []st.KeyedItemDescriptor{
			{Key: configOverrideKey, Item: sh.ConfigOverrideDescriptor(override)},
		}},
		{Kind: datakinds.Metrics, Items: []st.KeyedItemDescriptor{
			{Key: metricKey, Item: sh.MetricDescriptor(metric)},
		}},
	}
	dataSourceConfigurer := &mocks.ComponentConfigurerThatCapturesClientContext[ssys.DataSource]{
		Configurer: &mocks.DataSourceFactoryWithData{Data: data},
	}
	mockLog := ldlogtest.NewMockLog()
	config := ld.Config{
		DataStore:  ldcomponents.PersistentDataStore(dataStoreFactory).NoCaching(),
		DataSource: dataSourceConfigurer,
		Events:     ldcomponents.NoEvents(),
		Logging:    ldcomponents.Logging().Loggers(mockLog.Loggers),
	}

	client, err := ld.MakeCustomClient("sdk-key", config, 5*time.Second)
	require.NoError(t, err)
	defer client.Close() //nolint:errcheck
	dataSourceUpdateSink := dataSourceConfigurer.ReceivedClientContext.GetDataSourceUpdateSink()

	flagShouldHaveValueForUser := func(u ldcontext.Context, expectedValue ldvalue.Value) {
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
		dataSourceUpdateSink.Upsert(datakinds.Features, flagKey,
			sh.FlagDescriptor(flagv2))

		flagShouldHaveValueForUser(user, goodValue2)
		flagShouldHaveValueForUser(otherUser, badValue)
	})

	t.Run("update segment", func(t testbox.TestingT) {
		segmentv2 := makeSegmentThatMatchesUserKeys(2, userKey, otherUserKey)
		dataSourceUpdateSink.Upsert(datakinds.Segments, segmentKey,
			sh.SegmentDescriptor(segmentv2))
		flagShouldHaveValueForUser(otherUser, goodValue2) // otherUser is now matched by the segment
	})

	t.Run("delete segment", func(t testbox.TestingT) {
		// deleting the segment should cause the flag that uses it to stop matching
		dataSourceUpdateSink.Upsert(datakinds.Segments, segmentKey,
			st.ItemDescriptor{Version: 3, Item: nil})
		flagShouldHaveValueForUser(user, badValue)
	})

	t.Run("delete flag", func(t testbox.TestingT) {
		// deleting the flag should cause the flag to become unknown
		dataSourceUpdateSink.Upsert(datakinds.Features, flagKey,
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

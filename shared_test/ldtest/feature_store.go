// Package ldtest contains types and functions used by SDK unit tests in multiple packages.
//
// Application code should not use this package. In a future version, it will be moved to internal.
package ldtest

import (
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ld "gopkg.in/launchdarkly/go-server-sdk.v5"
)

// RunDataStoreTests runs a suite of tests on a data store.
// - makeStore: Creates a new data store instance, but does not call Init on it.
// - clearExistingData: If non-nil, this function will be called before each test to clear any storage
//   that the store instances may be sharing. If this is nil, it means store instances do not share any
//   common storage.
// - isCached: True if the instances returned by makeStore have caching enabled.
func RunDataStoreTests(t *testing.T, storeFactory ld.DataStoreFactory, clearExistingData func() error, isCached bool) {
	makeStore := func(t *testing.T) interfaces.DataStore {
		store, err := storeFactory(ld.Config{})
		require.NoError(t, err)
		return store
	}

	clearAll := func(t *testing.T) {
		if clearExistingData != nil {
			require.NoError(t, clearExistingData())
		}
	}

	initWithEmptyData := func(t *testing.T, store interfaces.DataStore) {
		err := store.Init(makeMockDataMap())
		require.NoError(t, err)
	}

	t.Run("store initialized after init", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		item1 := &MockDataItem{Key: "feature"}
		allData := makeMockDataMap(item1)
		require.NoError(t, store.Init(allData))

		assert.True(t, store.Initialized())
	})

	t.Run("init completely replaces previous data", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		item1 := &MockDataItem{Key: "first", Version: 1}
		item2 := &MockDataItem{Key: "second", Version: 1}
		otherItem1 := &MockOtherDataItem{Key: "first", Version: 1}
		allData := makeMockDataMap(item1, item2, otherItem1)
		require.NoError(t, store.Init(allData))

		items, err := store.All(MockData)
		require.NoError(t, err)
		assert.Equal(t, map[string]interfaces.VersionedData{item1.Key: item1, item2.Key: item2}, items)
		otherItems, err := store.All(MockOtherData)
		require.NoError(t, err)
		assert.Equal(t, map[string]interfaces.VersionedData{otherItem1.Key: otherItem1}, otherItems)

		otherItem2 := &MockOtherDataItem{Key: "second", Version: 1}
		allData = makeMockDataMap(item1, otherItem2)
		require.NoError(t, store.Init(allData))

		items, err = store.All(MockData)
		require.NoError(t, err)
		assert.Equal(t, map[string]interfaces.VersionedData{item1.Key: item1}, items)
		otherItems, err = store.All(MockOtherData)
		require.NoError(t, err)
		assert.Equal(t, map[string]interfaces.VersionedData{otherItem2.Key: otherItem2}, otherItems)
	})

	if !isCached && clearExistingData != nil {
		// Cannot run the following test in cached mode because the first false result will be cached.
		// Also, if clearExistingData is nil then this is the in-memory store and the test is meaningless.
		t.Run("one instance can detect if another instance has initialized the store", func(t *testing.T) {
			clearAll(t)
			store1 := makeStore(t)
			store2 := makeStore(t)

			assert.False(t, store1.Initialized())

			initWithEmptyData(t, store2)

			assert.True(t, store1.Initialized())
		})
	}

	t.Run("get existing item", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)
		item1 := &MockDataItem{Key: "feature"}
		assert.NoError(t, store.Upsert(MockData, item1))

		result, err := store.Get(MockData, item1.Key)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.Equal(t, result, item1)
	})

	t.Run("get nonexisting item", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		result, err := store.Get(MockData, "no") //nolint:megacheck // allow deprecated usage
		assert.Nil(t, result)
		assert.NoError(t, err)
	})

	t.Run("get all items", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		result, err := store.All(MockData)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.Len(t, result, 0)

		item1 := &MockDataItem{Key: "first", Version: 1}
		item2 := &MockDataItem{Key: "second", Version: 1}
		otherItem1 := &MockOtherDataItem{Key: "first", Version: 1}
		assert.NoError(t, store.Upsert(MockData, item1))
		assert.NoError(t, store.Upsert(MockData, item2))
		assert.NoError(t, store.Upsert(MockOtherData, otherItem1))

		result, err = store.All(MockData)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.Equal(t, map[string]interfaces.VersionedData{item1.Key: item1, item2.Key: item2}, result)
	})

	t.Run("upsert with newer version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		item1 := &MockDataItem{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(MockData, item1))

		item1a := &MockDataItem{Key: "feature", Version: item1.Version + 1}
		assert.NoError(t, store.Upsert(MockData, item1a))

		result, err := store.Get(MockData, item1.Key)
		assert.NoError(t, err)
		assert.Equal(t, item1a, result)
	})

	t.Run("upsert with older version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		item1 := &MockDataItem{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(MockData, item1))

		item1a := &MockDataItem{Key: "feature", Version: item1.Version - 1}
		assert.NoError(t, store.Upsert(MockData, item1a))

		result, err := store.Get(MockData, item1.Key)
		assert.NoError(t, err)
		assert.Equal(t, item1, result)
	})

	t.Run("delete with newer version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		item1 := &MockDataItem{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(MockData, item1))

		assert.NoError(t, store.Delete(MockData, item1.Key, item1.Version+1))

		result, err := store.Get(MockData, item1.Key)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("delete with older version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		item1 := &MockDataItem{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(MockData, item1))

		assert.NoError(t, store.Delete(MockData, item1.Key, item1.Version-1))

		result, err := store.Get(MockData, item1.Key)
		assert.NoError(t, err)
		assert.Equal(t, item1, result)
	})

	t.Run("delete unknown item", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		assert.NoError(t, store.Delete(MockData, "no", 1))

		result, err := store.Get(MockData, "no")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("upsert older version after delete", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		item1 := &MockDataItem{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(MockData, item1))

		assert.NoError(t, store.Delete(MockData, item1.Key, item1.Version+1))

		assert.NoError(t, store.Upsert(MockData, item1))

		result, err := store.Get(MockData, item1.Key)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

// RunDataStorePrefixIndependenceTests is for data store implementations that support
// storing independent data sets in the same database by assigning a different prefix/namespace
// to each one. It verifies that two store instances with different prefixes do not interfere
// with each other's data.
//
// makeStoreWithPrefix: Creates a DataStore instance with the specified prefix/namespace,
// which can be empty. All instances should use the same underlying database. The store
// should not have caching enabled.
//
// clearExistingData: Removes all data from the underlying store.
func RunDataStorePrefixIndependenceTests(t *testing.T,
	makeStoreWithPrefix func(string) (ld.DataStoreFactory, error),
	clearExistingData func() error) {

	runWithPrefixes := func(t *testing.T, name string, test func(*testing.T, interfaces.DataStore, interfaces.DataStore)) {
		err := clearExistingData()
		require.NoError(t, err)
		factory1, err := makeStoreWithPrefix("aaa")
		require.NoError(t, err)
		store1, err := factory1(ld.Config{Loggers: ldlog.NewDisabledLoggers()})
		require.NoError(t, err)
		factory2, err := makeStoreWithPrefix("bbb")
		require.NoError(t, err)
		store2, err := factory2(ld.Config{Loggers: ldlog.NewDisabledLoggers()})
		require.NoError(t, err)
		t.Run(name, func(t *testing.T) {
			test(t, store1, store2)
		})
	}

	runWithPrefixes(t, "Init", func(t *testing.T, store1 interfaces.DataStore, store2 interfaces.DataStore) {
		assert.False(t, store1.Initialized())
		assert.False(t, store2.Initialized())

		item1a := &MockDataItem{Key: "flag-a", Version: 1}
		item1b := &MockDataItem{Key: "flag-b", Version: 1}
		item2a := &MockDataItem{Key: "flag-a", Version: 2}
		item2c := &MockDataItem{Key: "flag-c", Version: 2}

		data1 := makeMockDataMap(item1a, item1b)
		data2 := makeMockDataMap(item2a, item2c)

		err := store1.Init(data1)
		require.NoError(t, err)

		assert.True(t, store1.Initialized())
		assert.False(t, store2.Initialized())

		err = store2.Init(data2)
		require.NoError(t, err)

		assert.True(t, store1.Initialized())
		assert.True(t, store2.Initialized())

		newItems1, err := store1.All(MockData)
		require.NoError(t, err)
		assert.Equal(t, data1[MockData], newItems1)

		newItem1a, err := store1.Get(MockData, item1a.Key)
		require.NoError(t, err)
		assert.Equal(t, item1a, newItem1a)

		newItem1b, err := store1.Get(MockData, item1b.Key)
		require.NoError(t, err)
		assert.Equal(t, item1b, newItem1b)

		newItems2, err := store2.All(MockData)
		require.NoError(t, err)
		assert.Equal(t, data2[MockData], newItems2)

		newItem2a, err := store2.Get(MockData, item2a.Key)
		require.NoError(t, err)
		assert.Equal(t, item2a, newItem2a)

		newItem2c, err := store2.Get(MockData, item2c.Key)
		require.NoError(t, err)
		assert.Equal(t, item2c, newItem2c)
	})

	runWithPrefixes(t, "Upsert/Delete", func(t *testing.T, store1 interfaces.DataStore, store2 interfaces.DataStore) {
		assert.False(t, store1.Initialized())
		assert.False(t, store2.Initialized())

		key := "flag"
		item1 := &MockDataItem{Key: key, Version: 1}
		item2 := &MockDataItem{Key: key, Version: 2}

		// Insert the one with the higher version first, so we can verify that the version-checking logic
		// is definitely looking in the right namespace
		err := store2.Upsert(MockData, item2)
		require.NoError(t, err)
		err = store1.Upsert(MockData, item1)
		require.NoError(t, err)

		newItem1, err := store1.Get(MockData, key)
		require.NoError(t, err)
		assert.Equal(t, item1, newItem1)

		newItem2, err := store2.Get(MockData, key)
		require.NoError(t, err)
		assert.Equal(t, item2, newItem2)

		err = store1.Delete(MockData, key, 2)
		require.NoError(t, err)

		newItem1a, err := store1.Get(MockData, key)
		require.NoError(t, err)
		assert.Nil(t, newItem1a)
	})
}

// RunDataStoreConcurrentModificationTests runs tests of concurrent modification behavior
// for store implementations that support testing this.
//
// store1: A DataStore instance.
//
// store2: A second DataStore instance which will be used to perform concurrent updates.
//
// setStore1UpdateHook: A function which, when called with another function as a parameter,
// will modify store1 so that it will call the latter function synchronously during each Upsert
// operation - after the old value has been read, but before the new one has been written.
func RunDataStoreConcurrentModificationTests(t *testing.T, factory1 ld.DataStoreFactory, factory2 ld.DataStoreFactory,
	setStore1UpdateHook func(func())) {

	config := ld.Config{Loggers: ldlog.NewDisabledLoggers()}
	store1, err := factory1(config)
	require.NoError(t, err)
	store2, err := factory2(config)
	require.NoError(t, err)

	key := "foo"

	makeItemWithVersion := func(version int) *MockDataItem {
		return &MockDataItem{Key: key, Version: version}
	}

	setupStore1 := func(initialVersion int) {
		allData := makeMockDataMap(makeItemWithVersion(initialVersion))
		require.NoError(t, store1.Init(allData))
	}

	setupConcurrentModifierToWriteVersions := func(versionsToWrite ...int) {
		i := 0
		setStore1UpdateHook(func() {
			if i < len(versionsToWrite) {
				newItem := makeItemWithVersion(versionsToWrite[i])
				err := store2.Upsert(MockData, newItem)
				require.NoError(t, err)
				i++
			}
		})
	}

	t.Run("upsert race condition against external client with lower version", func(t *testing.T) {
		setupStore1(1)
		setupConcurrentModifierToWriteVersions(2, 3, 4)

		assert.NoError(t, store1.Upsert(MockData, makeItemWithVersion(10)))

		var result interfaces.VersionedData
		result, err := store1.Get(MockData, key)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 10, result.GetVersion())
	})

	t.Run("upsert race condition against external client with lower version", func(t *testing.T) {
		setupStore1(1)
		setupConcurrentModifierToWriteVersions(3)

		assert.NoError(t, store1.Upsert(MockData, makeItemWithVersion(2)))

		var result interfaces.VersionedData
		result, err := store1.Get(MockData, key)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 3, result.GetVersion())
	})
}

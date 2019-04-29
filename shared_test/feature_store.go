package shared_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
)

// RunFeatureStoreTests runs a suite of tests on a feature store.
// - makeStore: Creates a new feature store instance, but does not call Init on it.
// - clearExistingData: If non-nil, this function will be called before each test to clear any storage
//   that the store instances may be sharing. If this is nil, it means store instances do not share any
//   common storage.
// - isCached: True if the instances returned by makeStore have caching enabled.
func RunFeatureStoreTests(t *testing.T, storeFactory func() (ld.FeatureStore, error), clearExistingData func() error, isCached bool) {
	makeStore := func(t *testing.T) ld.FeatureStore {
		store, err := storeFactory()
		require.NoError(t, err)
		return store
	}

	clearAll := func(t *testing.T) {
		if clearExistingData != nil {
			require.NoError(t, clearExistingData())
		}
	}

	initWithEmptyData := func(t *testing.T, store ld.FeatureStore) {
		err := store.Init(map[ld.VersionedDataKind]map[string]ld.VersionedData{ld.Features: make(map[string]ld.VersionedData)})
		require.NoError(t, err)
	}

	t.Run("store initialized after init", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		feature1 := ld.FeatureFlag{Key: "feature"}
		allData := makeAllVersionedDataMap(map[string]*ld.FeatureFlag{"feature": &feature1}, make(map[string]*ld.Segment))
		require.NoError(t, store.Init(allData))

		assert.True(t, store.Initialized())
	})

	t.Run("init completely replaces previous data", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		feature1 := ld.FeatureFlag{Key: "first", Version: 1}
		feature2 := ld.FeatureFlag{Key: "second", Version: 1}
		segment1 := ld.Segment{Key: "first", Version: 1}
		allData := makeAllVersionedDataMap(map[string]*ld.FeatureFlag{"first": &feature1, "second": &feature2},
			map[string]*ld.Segment{"first": &segment1})
		require.NoError(t, store.Init(allData))

		flags, err := store.All(ld.Features)
		require.NoError(t, err)
		segs, err := store.All(ld.Segments)
		require.NoError(t, err)
		assert.Equal(t, 1, flags["first"].GetVersion())
		assert.Equal(t, 1, flags["second"].GetVersion())
		assert.Equal(t, 1, segs["first"].GetVersion())

		feature1.Version = 2
		segment1.Version = 2
		allData = makeAllVersionedDataMap(map[string]*ld.FeatureFlag{"first": &feature1},
			map[string]*ld.Segment{"first": &segment1})
		require.NoError(t, store.Init(allData))

		flags, err = store.All(ld.Features)
		require.NoError(t, err)
		segs, err = store.All(ld.Segments)
		require.NoError(t, err)
		assert.Equal(t, 2, flags["first"].GetVersion())
		assert.Nil(t, flags["second"])
		assert.Equal(t, 2, segs["first"].GetVersion())
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

	t.Run("get existing feature", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)
		feature1 := ld.FeatureFlag{Key: "feature"}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NotNil(t, result)
		assert.NoError(t, err)

		if assert.IsType(t, &ld.FeatureFlag{}, result) {
			r := result.(*ld.FeatureFlag)
			assert.Equal(t, feature1.Key, r.Key)
		}
	})

	t.Run("get nonexisting feature", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		result, err := store.Get(ld.Features, "no")
		assert.Nil(t, result)
		assert.NoError(t, err)
	})

	t.Run("get all ld.Features", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		result, err := store.All(ld.Features)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.Len(t, result, 0)

		feature1 := ld.FeatureFlag{Key: "feature1"}
		feature2 := ld.FeatureFlag{Key: "feature2"}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))
		assert.NoError(t, store.Upsert(ld.Features, &feature2))

		result, err = store.All(ld.Features)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.Len(t, result, 2)

		if assert.IsType(t, &ld.FeatureFlag{}, result["feature1"]) {
			r := result["feature1"].(*ld.FeatureFlag)
			assert.Equal(t, "feature1", r.Key)
		}

		if assert.IsType(t, &ld.FeatureFlag{}, result["feature2"]) {
			r := result["feature2"].(*ld.FeatureFlag)
			assert.Equal(t, "feature2", r.Key)
		}
	})

	t.Run("upsert with newer version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		feature1a := ld.FeatureFlag{Key: "feature", Version: feature1.Version + 1}
		assert.NoError(t, store.Upsert(ld.Features, &feature1a))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)

		if assert.IsType(t, &ld.FeatureFlag{}, result) {
			r := result.(*ld.FeatureFlag)
			assert.Equal(t, feature1a.Version, r.Version)
		}
	})

	t.Run("upsert with older version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		feature1a := ld.FeatureFlag{Key: "feature", Version: feature1.Version - 1}
		assert.NoError(t, store.Upsert(ld.Features, &feature1a))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)

		if assert.IsType(t, &ld.FeatureFlag{}, result) {
			r := result.(*ld.FeatureFlag)
			assert.Equal(t, feature1.Version, r.Version)
		}
	})

	t.Run("delete with newer version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		assert.NoError(t, store.Delete(ld.Features, feature1.Key, feature1.Version+1))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("delete with older version", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		assert.NoError(t, store.Delete(ld.Features, feature1.Key, feature1.Version-1))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("delete unknown feature", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		assert.NoError(t, store.Delete(ld.Features, "no", 1))

		result, err := store.Get(ld.Features, "no")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("upsert older version after delete", func(t *testing.T) {
		clearAll(t)
		store := makeStore(t)
		initWithEmptyData(t, store)

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		assert.NoError(t, store.Delete(ld.Features, feature1.Key, feature1.Version+1))

		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

// RunFeatureStorePrefixIndependenceTests is for feature store implementations that support
// storing independent data sets in the same database by assigning a different prefix/namespace
// to each one. It verifies that two store instances with different prefixes do not interfere
// with each other's data.
//
// makeStoreWithPrefix: Creates a FeatureStore instance with the specified prefix/namespace,
// which can be empty. All instances should use the same underlying database. The store
// should not have caching enabled.
//
// clearExistingData: Removes all data from the underlying store.
func RunFeatureStorePrefixIndependenceTests(t *testing.T,
	makeStoreWithPrefix func(string) (ld.FeatureStore, error),
	clearExistingData func() error) {

	runWithPrefixes := func(t *testing.T, name string, test func(*testing.T, ld.FeatureStore, ld.FeatureStore)) {
		err := clearExistingData()
		require.NoError(t, err)
		store1, err := makeStoreWithPrefix("aaa")
		require.NoError(t, err)
		store2, err := makeStoreWithPrefix("bbb")
		require.NoError(t, err)
		t.Run(name, func(t *testing.T) {
			test(t, store1, store2)
		})
	}

	runWithPrefixes(t, "Init", func(t *testing.T, store1 ld.FeatureStore, store2 ld.FeatureStore) {
		assert.False(t, store1.Initialized())
		assert.False(t, store2.Initialized())

		flag1a := ld.FeatureFlag{Key: "flag-a", Version: 1}
		flag1b := ld.FeatureFlag{Key: "flag-b", Version: 1}
		flag2a := ld.FeatureFlag{Key: "flag-a", Version: 2}
		flag2c := ld.FeatureFlag{Key: "flag-c", Version: 2}

		data1 := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flag1a.Key: &flag1a, flag1b.Key: &flag1b},
		}
		data2 := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flag2a.Key: &flag2a, flag2c.Key: &flag2c},
		}

		err := store1.Init(data1)
		require.NoError(t, err)

		assert.True(t, store1.Initialized())
		assert.False(t, store2.Initialized())

		err = store2.Init(data2)
		require.NoError(t, err)

		assert.True(t, store1.Initialized())
		assert.True(t, store2.Initialized())

		newFlags1, err := store1.All(ld.Features)
		require.NoError(t, err)
		assert.Equal(t, data1[ld.Features], newFlags1)

		newFlag1a, err := store1.Get(ld.Features, flag1a.Key)
		require.NoError(t, err)
		assert.Equal(t, &flag1a, newFlag1a)

		newFlag1b, err := store1.Get(ld.Features, flag1b.Key)
		require.NoError(t, err)
		assert.Equal(t, &flag1b, newFlag1b)

		newFlags2, err := store2.All(ld.Features)
		require.NoError(t, err)
		assert.Equal(t, data2[ld.Features], newFlags2)

		newFlag2a, err := store2.Get(ld.Features, flag2a.Key)
		require.NoError(t, err)
		assert.Equal(t, &flag2a, newFlag2a)

		newFlag2c, err := store2.Get(ld.Features, flag2c.Key)
		require.NoError(t, err)
		assert.Equal(t, &flag2c, newFlag2c)
	})

	runWithPrefixes(t, "Upsert/Delete", func(t *testing.T, store1 ld.FeatureStore, store2 ld.FeatureStore) {
		assert.False(t, store1.Initialized())
		assert.False(t, store2.Initialized())

		flagKey := "flag"
		flag1 := ld.FeatureFlag{Key: flagKey, Version: 1}
		flag2 := ld.FeatureFlag{Key: flagKey, Version: 2}

		// Insert the one with the higher version first, so we can verify that the version-checking logic
		// is definitely looking in the right namespace
		err := store2.Upsert(ld.Features, &flag2)
		require.NoError(t, err)
		err = store1.Upsert(ld.Features, &flag1)
		require.NoError(t, err)

		newFlag1, err := store1.Get(ld.Features, flagKey)
		require.NoError(t, err)
		assert.Equal(t, &flag1, newFlag1)

		newFlag2, err := store2.Get(ld.Features, flagKey)
		require.NoError(t, err)
		assert.Equal(t, &flag2, newFlag2)

		err = store1.Delete(ld.Features, flagKey, 2)
		require.NoError(t, err)

		newFlag1a, err := store1.Get(ld.Features, flagKey)
		require.NoError(t, err)
		assert.Equal(t, nil, newFlag1a)
	})
}

// RunFeatureStoreConcurrentModificationTests runs tests of concurrent modification behavior
// for store implementations that support testing this.
//
// store1: A FeatureStore instance.
//
// store2: A second FeatureStore instance which will be used to perform concurrent updates.
//
// setStore1UpdateHook: A function which, when called with another function as a parameter,
// will modify store1 so that it will call the latter function synchronously during each Upsert
// operation - after the old value has been read, but before the new one has been written.
func RunFeatureStoreConcurrentModificationTests(t *testing.T, store1 ld.FeatureStore, store2 ld.FeatureStore,
	setStore1UpdateHook func(func())) {

	flagKey := "foo"

	makeFlagWithVersion := func(version int) *ld.FeatureFlag {
		return &ld.FeatureFlag{Key: flagKey, Version: version}
	}

	setupStore1 := func(initialVersion int) {
		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flagKey: makeFlagWithVersion(initialVersion)},
		}
		require.NoError(t, store1.Init(allData))
	}

	setupConcurrentModifierToWriteVersions := func(flagVersionsToWrite ...int) {
		i := 0
		setStore1UpdateHook(func() {
			if i < len(flagVersionsToWrite) {
				newFlag := makeFlagWithVersion(flagVersionsToWrite[i])
				err := store2.Upsert(ld.Features, newFlag)
				require.NoError(t, err)
				i++
			}
		})
	}

	t.Run("upsert race condition against external client with lower version", func(t *testing.T) {
		setupStore1(1)
		setupConcurrentModifierToWriteVersions(2, 3, 4)

		assert.NoError(t, store1.Upsert(ld.Features, makeFlagWithVersion(10)))

		var result ld.VersionedData
		result, err := store1.Get(ld.Features, flagKey)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 10, result.(*ld.FeatureFlag).Version)
	})

	t.Run("upsert race condition against external client with lower version", func(t *testing.T) {
		setupStore1(1)
		setupConcurrentModifierToWriteVersions(3)

		assert.NoError(t, store1.Upsert(ld.Features, makeFlagWithVersion(2)))

		var result ld.VersionedData
		result, err := store1.Get(ld.Features, flagKey)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 3, result.(*ld.FeatureFlag).Version)
	})
}

func makeAllVersionedDataMap(
	features map[string]*ld.FeatureFlag,
	segments map[string]*ld.Segment) map[ld.VersionedDataKind]map[string]ld.VersionedData {

	allData := make(map[ld.VersionedDataKind]map[string]ld.VersionedData)
	allData[ld.Features] = make(map[string]ld.VersionedData)
	allData[ld.Segments] = make(map[string]ld.VersionedData)
	for k, v := range features {
		allData[ld.Features][k] = v
	}
	for k, v := range segments {
		allData[ld.Segments][k] = v
	}
	return allData
}

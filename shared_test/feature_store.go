package shared_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ld "gopkg.in/launchdarkly/go-client.v4"
)

// RunFeatureStoreTests runs a suite of tests on a feature store
func RunFeatureStoreTests(t *testing.T, makeStore func() ld.FeatureStore) {
	var reinitStore = func() ld.FeatureStore {
		store := makeStore()
		err := store.Init(map[ld.VersionedDataKind]map[string]ld.VersionedData{ld.Features: make(map[string]ld.VersionedData)})
		assert.NoError(t, err, "store initialization failed")
		return store
	}

	t.Run("store initialized after init", func(t *testing.T) {
		store := reinitStore()
		feature1 := ld.FeatureFlag{Key: "feature"}
		allData := makeAllVersionedDataMap(map[string]*ld.FeatureFlag{"feature": &feature1}, make(map[string]*ld.Segment))
		assert.NoError(t, store.Init(allData))

		assert.True(t, store.Initialized())
	})

	t.Run("init completely replaces previous data", func(t *testing.T) {
		store := reinitStore()
		feature1 := ld.FeatureFlag{Key: "first", Version: 1}
		feature2 := ld.FeatureFlag{Key: "second", Version: 1}
		segment1 := ld.Segment{Key: "first", Version: 1}
		allData := makeAllVersionedDataMap(map[string]*ld.FeatureFlag{"first": &feature1, "second": &feature2},
			map[string]*ld.Segment{"first": &segment1})
		assert.NoError(t, store.Init(allData))

		flags, err := store.All(ld.Features)
		assert.NoError(t, err)
		segs, err := store.All(ld.Segments)
		assert.NoError(t, err)
		assert.Equal(t, 1, flags["first"].GetVersion())
		assert.Equal(t, 1, flags["second"].GetVersion())
		assert.Equal(t, 1, segs["first"].GetVersion())

		feature1.Version = 2
		segment1.Version = 2
		allData = makeAllVersionedDataMap(map[string]*ld.FeatureFlag{"first": &feature1},
			map[string]*ld.Segment{"first": &segment1})
		assert.NoError(t, store.Init(allData))

		flags, err = store.All(ld.Features)
		assert.NoError(t, err)
		segs, err = store.All(ld.Segments)
		assert.NoError(t, err)
		assert.Equal(t, 2, flags["first"].GetVersion())
		assert.Nil(t, flags["second"])
		assert.Equal(t, 2, segs["first"].GetVersion())
	})

	t.Run("get existing feature", func(t *testing.T) {
		store := reinitStore()
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
		store := reinitStore()

		result, err := store.Get(ld.Features, "no")
		assert.Nil(t, result)
		assert.NoError(t, err)
	})

	t.Run("get all ld.Features", func(t *testing.T) {
		store := reinitStore()

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
		store := reinitStore()

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
		store := reinitStore()

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
		store := reinitStore()

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		assert.NoError(t, store.Delete(ld.Features, feature1.Key, feature1.Version+1))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("delete with older version", func(t *testing.T) {
		store := reinitStore()

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		assert.NoError(t, store.Delete(ld.Features, feature1.Key, feature1.Version-1))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("delete unknown feature", func(t *testing.T) {
		store := reinitStore()

		assert.NoError(t, store.Delete(ld.Features, "no", 1))

		result, err := store.Get(ld.Features, "no")
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("upsert older version after delete", func(t *testing.T) {
		store := reinitStore()

		feature1 := ld.FeatureFlag{Key: "feature", Version: 10}
		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		assert.NoError(t, store.Delete(ld.Features, feature1.Key, feature1.Version+1))

		assert.NoError(t, store.Upsert(ld.Features, &feature1))

		result, err := store.Get(ld.Features, feature1.Key)
		assert.NoError(t, err)
		assert.Nil(t, result)
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
				store2.Upsert(ld.Features, newFlag)
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

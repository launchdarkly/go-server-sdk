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

	t.Run("init replaces all previous data", func(t *testing.T) {
		store := reinitStore()
		feature1 := ld.FeatureFlag{Key: "feature1"}
		feature2 := ld.FeatureFlag{Key: "feature2"}

		allData1 := makeAllVersionedDataMap(map[string]*ld.FeatureFlag{feature1.Key: &feature1, feature2.Key: &feature2},
			make(map[string]*ld.Segment))
		assert.NoError(t, store.Init(allData1))
		result, _ := store.Get(ld.Features, feature2.Key)
		assert.NotNil(t, result)

		allData2 := makeAllVersionedDataMap(map[string]*ld.FeatureFlag{feature1.Key: &feature1},
			make(map[string]*ld.Segment))
		assert.NoError(t, store.Init(allData2))
		result, _ = store.Get(ld.Features, feature2.Key)
		assert.Nil(t, result)
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

// RunFeatureStoreConcurrentModificationTests runs tests of concurrent modification behavior
// for store implementations that support testing this.
//
// The setConcurrentModifier function should behave as follows:
// - Install a hook in the feature store's Upsert logic that will be executed after the store has
//   read the previous value, but before it writes the new value.
// - When the hook is executed, try to read a flag from the provided channel. If this succeeds,
//   write the returned flag directly into the database. If the channel is closed, don't write
//   anything, and uninstall the hook.
func RunFeatureStoreConcurrentModificationTests(t *testing.T, store ld.FeatureStore,
	setConcurrentModifier func(<-chan ld.FeatureFlag)) {

	flagKey := "foo"

	makeFlagWithVersion := func(version int) *ld.FeatureFlag {
		return &ld.FeatureFlag{Key: flagKey, Version: version}
	}

	setupStore := func(initialVersion int) {
		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flagKey: makeFlagWithVersion(initialVersion)},
		}
		require.NoError(t, store.Init(allData))
	}

	makeChannelWithFlagVersions := func(versions ...int) chan ld.FeatureFlag {
		ch := make(chan ld.FeatureFlag, len(versions))
		for _, v := range versions {
			ch <- *makeFlagWithVersion(v)
		}
		close(ch)
		return ch
	}

	t.Run("upsert race condition against external client with lower version", func(t *testing.T) {
		setupStore(1)

		flagCh := makeChannelWithFlagVersions(2, 3, 4)
		setConcurrentModifier(flagCh)

		assert.NoError(t, store.Upsert(ld.Features, makeFlagWithVersion(10)))

		var result ld.VersionedData
		result, err := store.Get(ld.Features, flagKey)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 10, result.(*ld.FeatureFlag).Version)
	})

	t.Run("upsert race condition against external client with lower version", func(t *testing.T) {
		setupStore(1)

		flagCh := makeChannelWithFlagVersions(3)
		setConcurrentModifier(flagCh)

		assert.NoError(t, store.Upsert(ld.Features, makeFlagWithVersion(2)))

		var result ld.VersionedData
		result, err := store.Get(ld.Features, flagKey)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 3, result.(*ld.FeatureFlag).Version)
	})
}

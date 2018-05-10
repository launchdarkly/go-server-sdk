package shared_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ld "gopkg.in/launchdarkly/go-client.v3"
)

func RunFeatureStoreTests(t *testing.T, makeStore func() ld.FeatureStore) {
	var reinitStore = func() ld.FeatureStore {
		store := makeStore()
		store.Init(map[ld.VersionedDataKind]map[string]ld.VersionedData{ld.Features: make(map[string]ld.VersionedData)})
		return store
	}

	t.Run("store initialized after init", func(t *testing.T) {
		store := reinitStore()
		feature1 := ld.FeatureFlag{Key: "feature"}
		allData := makeAllVersionedDataMap(map[string]*ld.FeatureFlag{"feature": &feature1}, make(map[string]*ld.Segment))
		assert.NoError(t, store.Init(allData))

		assert.True(t, store.Initialized())
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

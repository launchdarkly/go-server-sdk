package ldclient

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func makeInMemoryStore() FeatureStore {
	return NewInMemoryFeatureStore(nil)
}

func RunFeatureStoreTests(t *testing.T, makeStore func() FeatureStore) {
	var reinitStore = func() FeatureStore {
		store := makeStore()
		store.Init(make(map[string]*FeatureFlag))
		return store
	}
	
	t.Run("store initialized after init", func(t *testing.T) {
		store := reinitStore()
		feature1 := FeatureFlag{Key: "feature"}
		allData := map[string]*FeatureFlag{"feature": &feature1}
		store.Init(allData)
		
		assert.True(t, store.Initialized())		
	})
	
	t.Run("get existing feature", func(t *testing.T) {
		store := reinitStore()
		feature1 := FeatureFlag{Key: "feature"}
		store.Upsert(feature1.Key, feature1)
		
		result, err := store.Get(feature1.Key)
		assert.NotNil(t, result)
		assert.Nil(t, err)
		assert.Equal(t, feature1.Key, result.Key)
	})
	
	t.Run("get nonexisting feature", func(t *testing.T) {
		store := reinitStore()
		
		result, err := store.Get("no")
		assert.Nil(t, result)
		assert.Nil(t, err)
	})
	
	t.Run("upsert with newer version", func(t *testing.T) {
		store := reinitStore()
		
		feature1 := FeatureFlag{Key: "feature", Version: 10}
		store.Upsert(feature1.Key, feature1)
		
		feature1a := FeatureFlag{Key: "feature", Version: feature1.Version + 1}
		store.Upsert(feature1.Key, feature1a)
		
		result, err := store.Get(feature1.Key)
		assert.Nil(t, err)
		assert.Equal(t, feature1a.Version, result.Version)
	})
	
	t.Run("upsert with older version", func(t *testing.T) {
		store := reinitStore()
		
		feature1 := FeatureFlag{Key: "feature", Version: 10}
		store.Upsert(feature1.Key, feature1)
		
		feature1a := FeatureFlag{Key: "feature", Version: feature1.Version - 1}
		store.Upsert(feature1.Key, feature1a)
		
		result, err := store.Get(feature1.Key)
		assert.Nil(t, err)
		assert.Equal(t, feature1.Version, result.Version)
	})
	
	t.Run("delete with newer version", func(t *testing.T) {
		store := reinitStore()
		
		feature1 := FeatureFlag{Key: "feature", Version: 10}
		store.Upsert(feature1.Key, feature1)
		
		store.Delete(feature1.Key, feature1.Version + 1)
		
		result, err := store.Get(feature1.Key)
		assert.Nil(t, err)
		assert.Nil(t, result)	
	})
	
	t.Run("delete with older version", func(t *testing.T) {
		store := reinitStore()
		
		feature1 := FeatureFlag{Key: "feature", Version: 10}
		store.Upsert(feature1.Key, feature1)
		
		store.Delete(feature1.Key, feature1.Version - 1)
		
		result, err := store.Get(feature1.Key)
		assert.Nil(t, err)
		assert.NotNil(t, result)
	})
	
	t.Run("delete unknown feature", func(t *testing.T) {
		store := reinitStore()
		
		store.Delete("no", 1)
		
		result, err := store.Get("no")
		assert.Nil(t, err)
		assert.Nil(t, result)
	})
	
	t.Run("upsert older version after delete", func(t *testing.T) {
		store := reinitStore()
		
		feature1 := FeatureFlag{Key: "feature", Version: 10}
		store.Upsert(feature1.Key, feature1)
		
		store.Delete(feature1.Key, feature1.Version + 1)
		
		store.Upsert(feature1.Key, feature1)
		
		result, err := store.Get(feature1.Key)
		assert.Nil(t, err)
		assert.Nil(t, result)
	})
}

func TestInMemoryFeatureStore(t *testing.T) {
	RunFeatureStoreTests(t, makeInMemoryStore)
}

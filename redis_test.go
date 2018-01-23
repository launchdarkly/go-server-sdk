package ldclient

import (
	"testing"
	"time"
)

func makeRedisStore() FeatureStore {
	return NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30 * time.Second, nil)
}

func TestRedisFeatureStore(t *testing.T) {
	RunFeatureStoreTests(t, makeRedisStore)
}

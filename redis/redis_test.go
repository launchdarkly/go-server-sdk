package redis

import (
	"testing"
	"time"

	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func TestRedisFeatureStoreUncached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, func() ld.FeatureStore {
		return NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
	})
}

func TestRedisFeatureStoreCached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, func() ld.FeatureStore {
		return NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 0, nil)
	})
}

func TestRedisFeatureStoreConcurrentModification(t *testing.T) {
	store1 := NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
	store2 := NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
	ldtest.RunFeatureStoreConcurrentModificationTests(t, store1, store2, func(hook func()) {
		store1.core.testTxHook = hook
	})
}

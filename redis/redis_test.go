package redis

import (
	"testing"
	"time"

	r "github.com/garyburd/redigo/redis"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

const redisURL = "redis://localhost:6379"

func TestRedisFeatureStoreUncached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeStoreWithCacheTTL(0), clearExistingData, false)
}

func TestRedisFeatureStoreCached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeStoreWithCacheTTL(30*time.Second), clearExistingData, true)
}

func TestRedisFeatureStoreConcurrentModification(t *testing.T) {
	store1 := NewRedisFeatureStoreFromUrl(redisURL, "", 0, nil)
	store2 := NewRedisFeatureStoreFromUrl(redisURL, "", 0, nil)
	ldtest.RunFeatureStoreConcurrentModificationTests(t, store1, store2, func(hook func()) {
		store1.core.testTxHook = hook
	})
}

func makeStoreWithCacheTTL(ttl time.Duration) func() ld.FeatureStore {
	return func() ld.FeatureStore {
		return NewRedisFeatureStoreFromUrl(redisURL, "", ttl, nil)
	}
}

func clearExistingData() error {
	client, err := r.DialURL(redisURL)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.Do("FLUSHDB")
	return err
}

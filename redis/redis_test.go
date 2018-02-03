package redis

import (
	"testing"
	"time"

	ld "gopkg.in/launchdarkly/go-client.v3"
	ldtest "gopkg.in/launchdarkly/go-client.v3/shared_test"
)

func makeRedisStore() ld.FeatureStore {
	return NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
}

func TestRedisFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeRedisStore)
}

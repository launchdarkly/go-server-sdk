package redis

import (
	"encoding/json"
	"testing"
	"time"

	r "github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func makeRedisStore() ld.FeatureStore {
	return NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
}

func TestRedisFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeRedisStore)
}

func TestRedisFeatureStoreConcurrentModification(t *testing.T) {
	store := NewRedisFeatureStoreFromUrl("redis://localhost:6379", "", 30*time.Second, nil)
	otherClient, err := r.DialURL("redis://localhost:6379")
	require.NoError(t, err)
	defer otherClient.Close()

	ldtest.RunFeatureStoreConcurrentModificationTests(t, store,
		func(flagGenerator func() *ld.FeatureFlag) {
			if flagGenerator == nil {
				store.testTxHook = nil
			} else {
				store.testTxHook = func() {
					f := flagGenerator()
					if f != nil {
						data, jsonErr := json.Marshal(f)
						require.NoError(t, jsonErr)
						_, err := otherClient.Do("HSET", "launchdarkly:features", f.Key, data)
						require.NoError(t, err)
					}
				}
			}
		})
}

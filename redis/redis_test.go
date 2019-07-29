package redis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	r "github.com/garyburd/redigo/redis"
	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
	ldtest "gopkg.in/launchdarkly/go-server-sdk.v4/shared_test/ldtest"
	"gopkg.in/launchdarkly/go-server-sdk.v4/utils"
)

const redisURL = "redis://localhost:6379"

func TestRedisFeatureStoreUncached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, func() (ld.FeatureStore, error) {
		return NewRedisFeatureStoreWithDefaults(CacheTTL(0), DialOptions(r.DialPassword("foobared")))
	}, clearExistingData, false)
}

// func TestRedisFeatureStoreUncachedWithDeprecatedConstructor(t *testing.T) {
// 	ldtest.RunFeatureStoreTests(t, func() (ld.FeatureStore, error) {
// 		return NewRedisFeatureStoreFromUrl(DefaultURL, "", 0, nil), nil
// 	}, clearExistingData, false)
// }

func TestRedisFeatureStoreCached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, func() (ld.FeatureStore, error) {
		return NewRedisFeatureStoreWithDefaults(CacheTTL(30*time.Second), DialOptions(r.DialPassword("foobared")))
	}, clearExistingData, true)
}

// func TestRedisFeatureStoreCachedWithDeprecatedConstructor(t *testing.T) {
// 	ldtest.RunFeatureStoreTests(t, func() (ld.FeatureStore, error) {
// 		return NewRedisFeatureStoreFromUrl(DefaultURL, "", 30*time.Second, nil), nil
// 	}, clearExistingData, true)
// }

func TestRedisFeatureStorePrefixes(t *testing.T) {
	ldtest.RunFeatureStorePrefixIndependenceTests(t,
		func(prefix string) (ld.FeatureStore, error) {
			return NewRedisFeatureStoreWithDefaults(Prefix(prefix), CacheTTL(0), DialOptions(r.DialPassword("foobared")))
		}, clearExistingData)
}

func TestRedisFeatureStoreConcurrentModification(t *testing.T) {
	core1, err := newRedisFeatureStoreInternal(DialOptions(r.DialPassword("foobared"))) // use the internal object so we can set testTxHook
	require.NoError(t, err)
	store1 := utils.NewFeatureStoreWrapper(core1)
	store2, err := NewRedisFeatureStoreWithDefaults(DialOptions(r.DialPassword("foobared")))
	require.NoError(t, err)
	ldtest.RunFeatureStoreConcurrentModificationTests(t, store1, store2, func(hook func()) {
		core1.testTxHook = hook
	})
}

func makeStoreWithCacheTTL(ttl time.Duration) func() (ld.FeatureStore, error) {
	return func() (ld.FeatureStore, error) {
		return NewRedisFeatureStoreFromUrl(redisURL, "", ttl, nil), nil
	}
}

func clearExistingData() error {
	client, err := r.DialURL(redisURL, r.DialPassword("foobared"))
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.Do("FLUSHDB")
	return err
}

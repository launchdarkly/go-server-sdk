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
	f, err := NewRedisFeatureStoreFactory(CacheTTL(0))
	require.NoError(t, err)
	ldtest.RunFeatureStoreTests(t, f, clearExistingData, false)
}

func TestRedisFeatureStoreCached(t *testing.T) {
	f, err := NewRedisFeatureStoreFactory(CacheTTL(30 * time.Second))
	require.NoError(t, err)
	ldtest.RunFeatureStoreTests(t, f, clearExistingData, true)
}

func TestRedisFeatureStorePrefixes(t *testing.T) {
	ldtest.RunFeatureStorePrefixIndependenceTests(t,
		func(prefix string) (ld.FeatureStoreFactory, error) {
			return NewRedisFeatureStoreFactory(Prefix(prefix), CacheTTL(0))
		}, clearExistingData)
}

func TestRedisFeatureStoreConcurrentModification(t *testing.T) {
	opts, err := validateOptions()
	require.NoError(t, err)
	var core1 *redisFeatureStoreCore
	factory1 := func(config ld.Config) (ld.FeatureStore, error) {
		core1 = newRedisFeatureStoreInternal(opts, config) // use the internal object so we can set testTxHook
		return utils.NewFeatureStoreWrapperWithConfig(core1, config), nil
	}
	factory2, err := NewRedisFeatureStoreFactory()
	require.NoError(t, err)
	ldtest.RunFeatureStoreConcurrentModificationTests(t, factory1, factory2, func(hook func()) {
		core1.testTxHook = hook
	})
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

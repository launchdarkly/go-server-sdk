package redis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	r "github.com/garyburd/redigo/redis"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	ldtest "gopkg.in/launchdarkly/go-server-sdk.v5/shared_test/ldtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/utils"
)

const redisURL = "redis://localhost:6379"

func TestRedisDataStoreUncached(t *testing.T) {
	f, err := NewRedisDataStoreFactory(CacheTTL(0))
	require.NoError(t, err)
	ldtest.RunDataStoreTests(t, f, clearExistingData, false)
}

func TestRedisDataStoreCached(t *testing.T) {
	f, err := NewRedisDataStoreFactory(CacheTTL(30 * time.Second))
	require.NoError(t, err)
	ldtest.RunDataStoreTests(t, f, clearExistingData, true)
}

func TestRedisDataStorePrefixes(t *testing.T) {
	ldtest.RunDataStorePrefixIndependenceTests(t,
		func(prefix string) (interfaces.DataStoreFactory, error) {
			return NewRedisDataStoreFactory(Prefix(prefix), CacheTTL(0))
		}, clearExistingData)
}

func TestRedisDataStoreConcurrentModification(t *testing.T) {
	opts, err := validateOptions()
	require.NoError(t, err)
	var core1 *redisDataStoreCore
	factory1 := func() (interfaces.DataStore, error) {
		core1 = newRedisDataStoreInternal(opts, ldlog.NewDisabledLoggers()) // use the internal object so we can set testTxHook
		return utils.NewDataStoreWrapperWithConfig(core1, ldlog.NewDisabledLoggers()), nil
	}
	factory2 := func() (interfaces.DataStore, error) {
		f, _ := NewRedisDataStoreFactory()
		return f.CreateDataStore(interfaces.NewClientContext("", nil, nil, ldlog.NewDisabledLoggers()))
	}
	require.NoError(t, err)
	ldtest.RunDataStoreConcurrentModificationTests(t, factory1, factory2, func(hook func()) {
		core1.testTxHook = hook
	})
}

func TestRedisStoreComponentTypeName(t *testing.T) {
	f, _ := NewRedisDataStoreFactory()
	store, _ := f.CreateDataStore(interfaces.NewClientContext("", nil, nil, ldlog.NewDisabledLoggers()))
	assert.Equal(t, "Redis", (store.(*utils.DataStoreWrapper)).GetDiagnosticsComponentTypeName())
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

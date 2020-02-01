package ldconsul

import (
	"testing"
	"time"

	c "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-server-sdk.v5"
	"gopkg.in/launchdarkly/go-server-sdk.v5/shared_test/ldtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/utils"
)

func TestConsulDataStoreUncached(t *testing.T) {
	ldtest.RunDataStoreTests(t, makeConsulStoreWithCacheTTL(0), clearExistingData, false)
}

func TestConsulDataStoreCached(t *testing.T) {
	ldtest.RunDataStoreTests(t, makeConsulStoreWithCacheTTL(30*time.Second), clearExistingData, true)
}

func TestConsulDataStorePrefixes(t *testing.T) {
	ldtest.RunDataStorePrefixIndependenceTests(t,
		func(prefix string) (ld.DataStoreFactory, error) {
			return NewConsulDataStoreFactory(Prefix(prefix), CacheTTL(0))
		}, clearExistingData)
}

func TestConsulDataStoreConcurrentModification(t *testing.T) {
	options, _ := validateOptions()
	var store1Core *dataStore
	factory1 := func(config ld.Config) (ld.DataStore, error) {
		store1Core, _ = newConsulDataStoreInternal(options, config) // we need the underlying implementation object so we can set testTxHook
		return utils.NewNonAtomicDataStoreWrapper(store1Core), nil
	}
	factory2, err := NewConsulDataStoreFactory()
	require.NoError(t, err)
	ldtest.RunDataStoreConcurrentModificationTests(t, factory1, factory2, func(hook func()) {
		store1Core.testTxHook = hook
	})
}

<<<<<<< HEAD
func makeConsulStoreWithCacheTTL(ttl time.Duration) ld.DataStoreFactory {
	f, _ := NewConsulDataStoreFactory(CacheTTL(ttl))
=======
func TestConsulStoreComponentTypeName(t *testing.T) {
	factory, _ := NewConsulFeatureStoreFactory()
	store, _ := factory(ld.DefaultConfig)
	assert.Equal(t, "Consul", (store.(*utils.FeatureStoreWrapper)).GetDiagnosticsComponentTypeName())
}

func makeConsulStoreWithCacheTTL(ttl time.Duration) ld.FeatureStoreFactory {
	f, _ := NewConsulFeatureStoreFactory(CacheTTL(ttl))
>>>>>>> eb/ch59296/remove-deprecated
	return f
}

func clearExistingData() error {
	client, err := c.NewClient(c.DefaultConfig())
	if err != nil {
		return err
	}
	kv := client.KV()
	_, err = kv.DeleteTree("", nil)
	return err
}

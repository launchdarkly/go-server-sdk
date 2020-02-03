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

func TestConsulFeatureStoreUncached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeConsulStoreWithCacheTTL(0), clearExistingData, false)
}

func TestConsulFeatureStoreCached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeConsulStoreWithCacheTTL(30*time.Second), clearExistingData, true)
}

func TestConsulFeatureStorePrefixes(t *testing.T) {
	ldtest.RunFeatureStorePrefixIndependenceTests(t,
		func(prefix string) (ld.FeatureStoreFactory, error) {
			return NewConsulFeatureStoreFactory(Prefix(prefix), CacheTTL(0))
		}, clearExistingData)
}

func TestConsulFeatureStoreConcurrentModification(t *testing.T) {
	options, _ := validateOptions()
	var store1Core *featureStore
	factory1 := func(config ld.Config) (ld.FeatureStore, error) {
		store1Core, _ = newConsulFeatureStoreInternal(options, config) // we need the underlying implementation object so we can set testTxHook
		return utils.NewNonAtomicFeatureStoreWrapper(store1Core), nil
	}
	factory2, err := NewConsulFeatureStoreFactory()
	require.NoError(t, err)
	ldtest.RunFeatureStoreConcurrentModificationTests(t, factory1, factory2, func(hook func()) {
		store1Core.testTxHook = hook
	})
}

func TestConsulStoreComponentTypeName(t *testing.T) {
	factory, _ := NewConsulFeatureStoreFactory()
	store, _ := factory(ld.DefaultConfig)
	assert.Equal(t, "Consul", (store.(*utils.FeatureStoreWrapper)).GetDiagnosticsComponentTypeName())
}

func makeConsulStoreWithCacheTTL(ttl time.Duration) ld.FeatureStoreFactory {
	f, _ := NewConsulFeatureStoreFactory(CacheTTL(ttl))
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

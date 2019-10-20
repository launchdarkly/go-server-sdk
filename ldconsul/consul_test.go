package ldconsul

import (
	"testing"
	"time"

	c "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
	"gopkg.in/launchdarkly/go-server-sdk.v4/shared_test/ldtest"
	"gopkg.in/launchdarkly/go-server-sdk.v4/utils"
)

func TestConsulFeatureStoreUncached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeConsulStoreWithCacheTTL(0), clearExistingData, false)
}

func TestConsulFeatureStoreCached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeConsulStoreWithCacheTTL(30*time.Second), clearExistingData, true)
}

func TestConsulFeatureStorePrefixes(t *testing.T) {
	ldtest.RunFeatureStorePrefixIndependenceTests(t,
		func(prefix string) (ld.FeatureStore, error) {
			return NewConsulFeatureStore(Prefix(prefix), CacheTTL(0))
		}, clearExistingData)
}

func TestConsulFeatureStoreConcurrentModification(t *testing.T) {
	options, _ := validateOptions()
	store1Core, err := newConsulFeatureStoreInternal(options, ld.Config{}) // we need the underlying implementation object so we can set testTxHook
	require.NoError(t, err)
	store1 := utils.NewNonAtomicFeatureStoreWrapper(store1Core)
	store2, err := NewConsulFeatureStore()
	require.NoError(t, err)

	ldtest.RunFeatureStoreConcurrentModificationTests(t, store1, store2, func(hook func()) {
		store1Core.testTxHook = hook
	})
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

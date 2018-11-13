package ldconsul

import (
	"testing"
	"time"

	c "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
	"gopkg.in/launchdarkly/go-client.v4/utils"
)

func TestConsulFeatureStoreUncached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeConsulStoreWithCacheTTL(0), clearExistingData, false)
}

func TestConsulFeatureStoreCached(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeConsulStoreWithCacheTTL(30*time.Second), clearExistingData, true)
}

func TestConsulFeatureStoreConcurrentModification(t *testing.T) {
	store1Core, err := newConsulFeatureStoreInternal() // we need the underlying implementation object so we can set testTxHook
	require.NoError(t, err)
	store1 := utils.NewFeatureStoreWrapper(store1Core)
	store2, err := NewConsulFeatureStore()
	require.NoError(t, err)

	ldtest.RunFeatureStoreConcurrentModificationTests(t, store1, store2, func(hook func()) {
		store1Core.testTxHook = hook
	})
}

func makeConsulStoreWithCacheTTL(ttl time.Duration) func() (ld.FeatureStore, error) {
	return func() (ld.FeatureStore, error) {
		return NewConsulFeatureStore(CacheTTL(ttl))
	}
}

func clearExistingData() error {
	client, err := c.NewClient(c.DefaultConfig())
	if err != nil {
		return err
	}
	kv := client.KV()
	_, err = kv.DeleteTree(DefaultPrefix, nil)
	return err
}

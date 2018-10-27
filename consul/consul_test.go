package consul

import (
	"encoding/json"
	"testing"

	consul "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func TestConsulFeatureStore(t *testing.T) {
	makeConsulStore := func() ld.FeatureStore {
		store, err := NewConsulFeatureStore()
		require.NoError(t, err)
		return store
	}
	ldtest.RunFeatureStoreTests(t, makeConsulStore)
}

func TestConsulFeatureStoreConcurrentModification(t *testing.T) {
	store, err := newConsulFeatureStoreInternal() // we need the underlying implementation object so we can set testTxHook
	require.NoError(t, err)
	otherClient, err := consul.NewClient(&consul.Config{})
	require.NoError(t, err)

	ldtest.RunFeatureStoreConcurrentModificationTests(t, store,
		func(flagCh <-chan ld.FeatureFlag) {
			store.testTxHook = func() {
				if f, ok := <-flagCh; ok {
					data, jsonErr := json.Marshal(f)
					require.NoError(t, jsonErr)
					pair := &consul.KVPair{
						Key:   store.featureKeyFor(ld.Features, f.Key),
						Value: data,
					}
					_, err := otherClient.KV().Put(pair, nil)
					require.NoError(t, err)
				} else {
					store.testTxHook = nil
				}
			}
		})
}

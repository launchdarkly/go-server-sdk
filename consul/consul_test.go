package consul

import (
	"testing"

	consul "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func TestConsulFeatureStore(t *testing.T) {
	makeConsulStore := func() ld.FeatureStore {
		config := consul.Config{}
		store, err := NewConsulFeatureStoreWithConfig(&config, "", 0, nil)
		require.NoError(t, err)
		return store
	}
	ldtest.RunFeatureStoreTests(t, makeConsulStore)
}

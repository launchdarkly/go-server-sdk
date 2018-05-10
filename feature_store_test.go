package ldclient_test

import (
	"testing"

	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func makeInMemoryStore() ld.FeatureStore {
	return ld.NewInMemoryFeatureStore(nil)
}

func TestInMemoryFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeInMemoryStore)
}

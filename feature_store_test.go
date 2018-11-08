package ldclient_test

import (
	"testing"

	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func makeInMemoryStore() (ld.FeatureStore, error) {
	return ld.NewInMemoryFeatureStore(nil), nil
}

func TestInMemoryFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeInMemoryStore, nil, false)
}

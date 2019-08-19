package ldclient_test

import (
	"testing"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
	"gopkg.in/launchdarkly/go-server-sdk.v4/shared_test/ldtest"
)

func makeInMemoryStore() (ld.FeatureStore, error) {
	return ld.NewInMemoryFeatureStore(nil), nil
}

func TestInMemoryFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeInMemoryStore, nil, false)
}

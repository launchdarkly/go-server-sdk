package ldclient_test

import (
	"testing"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
	"gopkg.in/launchdarkly/go-server-sdk.v4/shared_test/ldtest"
)

func TestInMemoryFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, ld.NewInMemoryFeatureStoreFactory(), nil, false)
}

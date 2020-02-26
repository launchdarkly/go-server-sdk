package ldcomponents

import (
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v5/shared_test/ldtest"
)

func TestInMemoryDataStore(t *testing.T) {
	ldtest.RunDataStoreTests(t, InMemoryDataStore(), nil, false)
}

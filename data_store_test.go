package ldclient_test

import (
	"testing"

	ld "gopkg.in/launchdarkly/go-server-sdk.v5"
	"gopkg.in/launchdarkly/go-server-sdk.v5/shared_test/ldtest"
)

func TestInMemoryDataStore(t *testing.T) {
	ldtest.RunDataStoreTests(t, ld.NewInMemoryDataStoreFactory(), nil, false)
}

package ldclient_test

import (
	"testing"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
	"gopkg.in/launchdarkly/go-server-sdk.v4/shared_test/ldtest"
)

func TestInMemoryDataStore(t *testing.T) {
	ldtest.RunDataStoreTests(t, ld.NewInMemoryDataStoreFactory(), nil, false)
}

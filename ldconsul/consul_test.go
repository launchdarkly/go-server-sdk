package ldconsul

import (
	"testing"

	c "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/testhelpers"
)

func TestConsulDataStore(t *testing.T) {
	testhelpers.NewPersistentDataStoreTestSuite(makeTestStore, clearTestData).
		ErrorStoreFactory(makeFailedStore(), verifyFailedStoreError).
		ConcurrentModificationHook(setConcurrentModificationHook).
		Run(t)
}

func makeTestStore(prefix string) interfaces.PersistentDataStoreFactory {
	return DataStore().Prefix(prefix)
}

func makeFailedStore() interfaces.PersistentDataStoreFactory {
	// Here we ensure that all Consul operations will fail by providing an invalid hostname.
	return DataStore().Address("not-a-real-consul-host")
}

func verifyFailedStoreError(t assert.TestingT, err error) {
	assert.Contains(t, err.Error(), "no such host")
}

func clearTestData(prefix string) error {
	client, err := c.NewClient(c.DefaultConfig())
	if err != nil {
		return err
	}
	kv := client.KV()
	_, err = kv.DeleteTree(prefix+"/", nil)
	return err
}

func setConcurrentModificationHook(store interfaces.PersistentDataStore, hook func()) {
	store.(*consulDataStoreImpl).testTxHook = hook
}

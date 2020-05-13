package ldconsul

import (
	"testing"

	c "github.com/hashicorp/consul/api"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func TestConsulDataStore(t *testing.T) {
	sharedtest.NewPersistentDataStoreTestSuite(makeTestStore, clearTestData).
		ConcurrentModificationHook(setConcurrentModificationHook).
		Run(t)
}

func makeTestStore(prefix string) interfaces.PersistentDataStoreFactory {
	return DataStore().Prefix(prefix)
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

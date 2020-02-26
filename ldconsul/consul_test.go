package ldconsul

import (
	"testing"
	"time"

	c "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/shared_test/ldtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/utils"
)

func TestConsulDataStoreUncached(t *testing.T) {
	ldtest.RunDataStoreTests(t, makeConsulStoreWithCacheTTL(0), clearExistingData, false)
}

func TestConsulDataStoreCached(t *testing.T) {
	ldtest.RunDataStoreTests(t, makeConsulStoreWithCacheTTL(30*time.Second), clearExistingData, true)
}

func TestConsulDataStorePrefixes(t *testing.T) {
	ldtest.RunDataStorePrefixIndependenceTests(t,
		func(prefix string) (interfaces.DataStoreFactory, error) {
			return NewConsulDataStoreFactory(Prefix(prefix), CacheTTL(0))
		}, clearExistingData)
}

func TestConsulDataStoreConcurrentModification(t *testing.T) {
	options, _ := validateOptions()
	var store1Core *dataStore
	factory1 := func() (interfaces.DataStore, error) {
		store1Core, _ = newConsulDataStoreInternal(options, ldlog.NewDisabledLoggers()) // we need the underlying implementation object so we can set testTxHook
		return utils.NewNonAtomicDataStoreWrapperWithConfig(store1Core, ldlog.NewDisabledLoggers()), nil
	}
	factory2 := func() (interfaces.DataStore, error) {
		f, _ := NewConsulDataStoreFactory()
		return f.CreateDataStore(interfaces.NewClientContext("", nil, nil, ldlog.NewDisabledLoggers()))
	}
	ldtest.RunDataStoreConcurrentModificationTests(t, factory1, factory2, func(hook func()) {
		store1Core.testTxHook = hook
	})
}

func TestConsulStoreComponentTypeName(t *testing.T) {
	factory, _ := NewConsulDataStoreFactory()
	assert.Equal(t, "Consul", (factory.(consulDataStoreFactory)).GetDiagnosticsComponentTypeName())
}

func makeConsulStoreWithCacheTTL(ttl time.Duration) interfaces.DataStoreFactory {
	f, _ := NewConsulDataStoreFactory(CacheTTL(ttl))
	return f
}

func clearExistingData() error {
	client, err := c.NewClient(c.DefaultConfig())
	if err != nil {
		return err
	}
	kv := client.KV()
	_, err = kv.DeleteTree("", nil)
	return err
}

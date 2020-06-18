package ldconsul

import (
	"testing"

	c "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func TestDataStoreBuilder(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		b := DataStore()
		assert.Equal(t, c.Config{}, b.consulConfig)
		assert.Equal(t, DefaultPrefix, b.prefix)
	})

	t.Run("Address", func(t *testing.T) {
		b := DataStore().Address("a")
		assert.Equal(t, "a", b.consulConfig.Address)
	})

	t.Run("Config", func(t *testing.T) {
		var config c.Config
		config.Address = "a"

		b := DataStore().Config(config)
		assert.Equal(t, config, b.consulConfig)
	})

	t.Run("Prefix", func(t *testing.T) {
		b := DataStore().Prefix("p")
		assert.Equal(t, "p", b.prefix)

		b.Prefix("")
		assert.Equal(t, DefaultPrefix, b.prefix)
	})

	t.Run("error for invalid address", func(t *testing.T) {
		b := DataStore().Address("bad-scheme://no")
		_, err := b.CreatePersistentDataStore(sharedtest.NewSimpleTestContext(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Unknown protocol")
	})
}

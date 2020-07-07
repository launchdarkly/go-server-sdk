package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

type clientOfflineTestParams struct {
	client  *LDClient
	store   interfaces.DataStore
	mockLog *ldlogtest.MockLog
}

func withClientOfflineTestParams(callback func(clientExternalUpdatesTestParams)) {
	p := clientExternalUpdatesTestParams{}
	p.store = datastore.NewInMemoryDataStore(ldlog.NewDisabledLoggers())
	p.mockLog = ldlogtest.NewMockLog()
	config := Config{
		Offline:   true,
		DataStore: sharedtest.SingleDataStoreFactory{Instance: p.store},
		Logging:   ldcomponents.Logging().Loggers(p.mockLog.Loggers),
	}
	p.client, _ = MakeCustomClient("sdk_key", config, 0)
	defer p.client.Close()
	callback(p)
}

func TestClientOfflineMode(t *testing.T) {
	t.Run("is initialized", func(t *testing.T) {
		withClientOfflineTestParams(func(p clientExternalUpdatesTestParams) {
			assert.True(t, p.client.Initialized())
			assert.Equal(t, interfaces.DataSourceStateValid,
				p.client.GetDataSourceStatusProvider().GetStatus().State)
		})
	})

	t.Run("reports offline status", func(t *testing.T) {
		withClientOfflineTestParams(func(p clientExternalUpdatesTestParams) {
			assert.True(t, p.client.IsOffline())
		})
	})

	t.Run("logs appropriate message at startup", func(t *testing.T) {
		withClientOfflineTestParams(func(p clientExternalUpdatesTestParams) {
			assert.Contains(
				t,
				p.mockLog.GetOutput(ldlog.Info),
				"Starting LaunchDarkly client in offline mode",
			)
		})
	})

	t.Run("returns default values", func(t *testing.T) {
		withClientOfflineTestParams(func(p clientExternalUpdatesTestParams) {
			result, err := p.client.BoolVariation("flagkey", evalTestUser, false)
			assert.NoError(t, err)
			assert.False(t, result)
		})
	})

	t.Run("returns invalid state from AllFlagsState", func(t *testing.T) {
		withClientOfflineTestParams(func(p clientExternalUpdatesTestParams) {
			result := p.client.AllFlagsState(evalTestUser)
			assert.False(t, result.IsValid())
		})
	})
}

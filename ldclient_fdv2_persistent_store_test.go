package ldclient

import (
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdatav2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The SDK must write the updates it received during a persistent store outage back to the store
// once the store recovers, so that other readers of that store see them.
func TestFDV2PersistentStoreIsRewrittenAfterOutage(t *testing.T) {
	for _, cache := range []struct {
		name string
		ttl  time.Duration
	}{
		{"finite cache TTL", 30 * time.Second},
		{"infinite cache TTL", -1},
		{"no cache", 0},
	} {
		t.Run(cache.name, func(t *testing.T) {
			core := mocks.NewMockPersistentDataStore()
			td := ldtestdatav2.DataSource()
			td.Update(td.Flag("flag").On(true))

			store := ldcomponents.PersistentDataStore(
				mocks.SingleComponentConfigurer[subsystems.PersistentDataStore]{Instance: core},
			).CacheTime(cache.ttl)

			config := Config{
				Events:  ldcomponents.NoEvents(),
				Logging: ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
				DataSystem: ldcomponents.DataSystem().Custom().
					Synchronizers(td).
					DataStore(store, subsystems.DataStoreModeReadWrite),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()
			require.Equal(t, 1, core.ForceGet(datakinds.Features, "flag").Version)

			// Take the store down, then change the flag. The SDK applies the update in memory, but
			// cannot write it to the store.
			core.SetFakeError(errors.New("store is down"))
			core.SetAvailable(false)
			td.Update(td.Flag("flag").On(false))
			require.Eventually(t, func() bool {
				value, _ := client.BoolVariation("flag", testUser, true)
				return !value
			}, time.Second, 10*time.Millisecond, "the SDK should have applied the update in memory")

			// Bring the store back up.
			core.SetFakeError(nil)
			core.SetAvailable(true)

			assert.Eventually(t, func() bool {
				return core.ForceGet(datakinds.Features, "flag").Version == 2
			}, 2*time.Second, 50*time.Millisecond, "the store should have been rewritten after recovery")
		})
	}
}

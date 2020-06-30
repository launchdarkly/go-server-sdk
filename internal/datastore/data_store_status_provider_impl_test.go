package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

type dataStoreStatusProviderTestParams struct {
	dataStore               *sharedtest.CapturingDataStore
	dataStoreUpdates        interfaces.DataStoreUpdates
	dataStoreStatusProvider interfaces.DataStoreStatusProvider
}

func dataStoreStatusProviderTest(action func(dataStoreStatusProviderTestParams)) {
	p := dataStoreStatusProviderTestParams{}
	p.dataStore = sharedtest.NewCapturingDataStore(NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	broadcaster := internal.NewDataStoreStatusBroadcaster()
	defer broadcaster.Close()
	dataStoreUpdates := NewDataStoreUpdatesImpl(broadcaster)
	p.dataStoreUpdates = dataStoreUpdates
	p.dataStoreStatusProvider = NewDataStoreStatusProviderImpl(p.dataStore, dataStoreUpdates)

	action(p)
}

func TestDataStoreStatusProviderImpl(t *testing.T) {
	t.Run("GetStatus", func(t *testing.T) {
		dataStoreStatusProviderTest(func(p dataStoreStatusProviderTestParams) {
			assert.Equal(t, interfaces.DataStoreStatus{Available: true}, p.dataStoreStatusProvider.GetStatus())

			newStatus := interfaces.DataStoreStatus{Available: false}
			p.dataStoreUpdates.UpdateStatus(newStatus)

			assert.Equal(t, newStatus, p.dataStoreStatusProvider.GetStatus())
		})
	})

	t.Run("IsStatusMonitoringEnabled", func(t *testing.T) {
		dataStoreStatusProviderTest(func(p dataStoreStatusProviderTestParams) {
			p.dataStore.SetStatusMonitoringEnabled(true)
			assert.True(t, p.dataStoreStatusProvider.IsStatusMonitoringEnabled())
		})

		dataStoreStatusProviderTest(func(p dataStoreStatusProviderTestParams) {
			p.dataStore.SetStatusMonitoringEnabled(false)
			assert.False(t, p.dataStoreStatusProvider.IsStatusMonitoringEnabled())
		})
	})

	t.Run("listeners", func(t *testing.T) {
		dataStoreStatusProviderTest(func(p dataStoreStatusProviderTestParams) {
			ch1 := p.dataStoreStatusProvider.AddStatusListener()
			ch2 := p.dataStoreStatusProvider.AddStatusListener()
			ch3 := p.dataStoreStatusProvider.AddStatusListener()
			p.dataStoreStatusProvider.RemoveStatusListener(ch2)

			newStatus := interfaces.DataStoreStatus{Available: false}
			p.dataStoreUpdates.UpdateStatus(newStatus)

			require.Len(t, ch1, 1)
			require.Len(t, ch2, 0)
			require.Len(t, ch3, 1)
			assert.Equal(t, newStatus, <-ch1)
			assert.Equal(t, newStatus, <-ch3)
		})
	})
}

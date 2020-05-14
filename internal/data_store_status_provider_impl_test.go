package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

func TestDataStoreStatusProviderImpl(t *testing.T) {
	t.Run("GetStatus", func(t *testing.T) {
		_, dataStoreUpdates, dataStoreStatusProvider := makeDataStoreStatusProviderTestComponents()

		assert.Equal(t, interfaces.DataStoreStatus{Available: true}, dataStoreStatusProvider.GetStatus())

		newStatus := interfaces.DataStoreStatus{Available: false}
		dataStoreUpdates.UpdateStatus(newStatus)

		assert.Equal(t, newStatus, dataStoreStatusProvider.GetStatus())
	})

	t.Run("IsStatusMonitoringEnabled", func(t *testing.T) {
		store1, _, dataStoreStatusProvider1 := makeDataStoreStatusProviderTestComponents()
		store1.statusMonitoringEnabled = true

		assert.True(t, dataStoreStatusProvider1.IsStatusMonitoringEnabled())

		store2, _, dataStoreStatusProvider2 := makeDataStoreStatusProviderTestComponents()
		store2.statusMonitoringEnabled = false

		assert.False(t, dataStoreStatusProvider2.IsStatusMonitoringEnabled())
	})

	t.Run("listeners", func(t *testing.T) {
		_, dataStoreUpdates, dataStoreStatusProvider := makeDataStoreStatusProviderTestComponents()

		ch1 := dataStoreStatusProvider.AddStatusListener()
		ch2 := dataStoreStatusProvider.AddStatusListener()
		ch3 := dataStoreStatusProvider.AddStatusListener()
		dataStoreStatusProvider.RemoveStatusListener(ch2)

		newStatus := interfaces.DataStoreStatus{Available: false}
		dataStoreUpdates.UpdateStatus(newStatus)

		require.Len(t, ch1, 1)
		require.Len(t, ch2, 0)
		require.Len(t, ch3, 1)
		assert.Equal(t, newStatus, <-ch1)
		assert.Equal(t, newStatus, <-ch3)
	})
}

func makeDataStoreStatusProviderTestComponents() (*dataStoreWithStatusMonitoringFlag, interfaces.DataStoreUpdates, interfaces.DataStoreStatusProvider) {
	store := &dataStoreWithStatusMonitoringFlag{}
	dataStoreUpdates := NewDataStoreUpdatesImpl(NewDataStoreStatusBroadcaster())
	dataStoreStatusProvider := NewDataStoreStatusProviderImpl(store, dataStoreUpdates)
	return store, dataStoreUpdates, dataStoreStatusProvider
}

type dataStoreWithStatusMonitoringFlag struct {
	statusMonitoringEnabled bool
}

func (d *dataStoreWithStatusMonitoringFlag) Init(allData []interfaces.StoreCollection) error {
	return nil
}

func (d *dataStoreWithStatusMonitoringFlag) Get(kind interfaces.StoreDataKind, key string) (interfaces.StoreItemDescriptor, error) {
	return interfaces.StoreItemDescriptor{Version: -1, Item: nil}, nil
}

func (d *dataStoreWithStatusMonitoringFlag) GetAll(kind interfaces.StoreDataKind) ([]interfaces.StoreKeyedItemDescriptor, error) {
	return nil, nil
}

func (d *dataStoreWithStatusMonitoringFlag) Upsert(kind interfaces.StoreDataKind, key string, newItem interfaces.StoreItemDescriptor) error {
	return nil
}

func (d *dataStoreWithStatusMonitoringFlag) IsInitialized() bool {
	return false
}

func (d *dataStoreWithStatusMonitoringFlag) IsStatusMonitoringEnabled() bool {
	return d.statusMonitoringEnabled
}

func (d *dataStoreWithStatusMonitoringFlag) Close() error {
	return nil
}

package internal

import "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

type DataStoreStatusProviderImpl struct {
	store            interfaces.DataStore
	dataStoreUpdates *DataStoreUpdatesImpl
}

func NewDataStoreStatusProviderImpl(
	store interfaces.DataStore,
	dataStoreUpdates *DataStoreUpdatesImpl,
) *DataStoreStatusProviderImpl {
	return &DataStoreStatusProviderImpl{
		store:            store,
		dataStoreUpdates: dataStoreUpdates,
	}
}

func (d *DataStoreStatusProviderImpl) GetStatus() interfaces.DataStoreStatus {
	return d.dataStoreUpdates.getStatus()
}

func (d *DataStoreStatusProviderImpl) IsStatusMonitoringEnabled() bool {
	return d.store.IsStatusMonitoringEnabled()
}

func (d *DataStoreStatusProviderImpl) AddStatusListener() <-chan interfaces.DataStoreStatus {
	return d.dataStoreUpdates.getBroadcaster().AddListener()
}

func (d *DataStoreStatusProviderImpl) RemoveStatusListener(ch <-chan interfaces.DataStoreStatus) {
	d.dataStoreUpdates.getBroadcaster().RemoveListener(ch)
}

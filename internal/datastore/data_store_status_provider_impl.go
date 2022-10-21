package datastore

import (
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// dataStoreStatusProviderImpl is the internal implementation of DataStoreStatusProvider. It's not
// exported because the rest of the SDK code only interacts with the public interface.
type dataStoreStatusProviderImpl struct {
	store            subsystems.DataStore
	dataStoreUpdates *DataStoreUpdateSinkImpl
}

// NewDataStoreStatusProviderImpl creates the internal implementation of DataStoreStatusProvider.
func NewDataStoreStatusProviderImpl(
	store subsystems.DataStore,
	dataStoreUpdates *DataStoreUpdateSinkImpl,
) interfaces.DataStoreStatusProvider {
	return &dataStoreStatusProviderImpl{
		store:            store,
		dataStoreUpdates: dataStoreUpdates,
	}
}

func (d *dataStoreStatusProviderImpl) GetStatus() interfaces.DataStoreStatus {
	return d.dataStoreUpdates.getStatus()
}

func (d *dataStoreStatusProviderImpl) IsStatusMonitoringEnabled() bool {
	return d.store.IsStatusMonitoringEnabled()
}

func (d *dataStoreStatusProviderImpl) AddStatusListener() <-chan interfaces.DataStoreStatus {
	return d.dataStoreUpdates.getBroadcaster().AddListener()
}

func (d *dataStoreStatusProviderImpl) RemoveStatusListener(ch <-chan interfaces.DataStoreStatus) {
	d.dataStoreUpdates.getBroadcaster().RemoveListener(ch)
}

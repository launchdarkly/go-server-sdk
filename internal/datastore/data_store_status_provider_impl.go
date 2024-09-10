package datastore

import (
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

type StatusMonitorable interface {
	// IsStatusMonitoringEnabled returns true if this data store implementation supports status
	// monitoring.
	//
	// This is normally only true for persistent data stores created with ldcomponents.PersistentDataStore(),
	// but it could also be true for any custom DataStore implementation that makes use of the
	// statusUpdater parameter provided to the DataStoreFactory. Returning true means that the store
	// guarantees that if it ever enters an invalid state (that is, an operation has failed or it knows
	// that operations cannot succeed at the moment), it will publish a status update, and will then
	// publish another status update once it has returned to a valid state.
	//
	// The same value will be returned from DataStoreStatusProvider.IsStatusMonitoringEnabled().
	IsStatusMonitoringEnabled() bool
}

// dataStoreStatusProviderImpl is the internal implementation of DataStoreStatusProvider. It's not
// exported because the rest of the SDK code only interacts with the public interface.
type dataStoreStatusProviderImpl struct {
	store            StatusMonitorable
	dataStoreUpdates *DataStoreUpdateSinkImpl
}

// NewDataStoreStatusProviderImpl creates the internal implementation of DataStoreStatusProvider.
func NewDataStoreStatusProviderImpl(
	store StatusMonitorable,
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

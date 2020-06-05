package internal

import (
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// dataSourceStatusProviderImpl is the internal implementation of DataSourceStatusProvider. It's not
// exported because the rest of the SDK code only interacts with the public interface.
type dataSourceStatusProviderImpl struct {
	broadcaster       *DataSourceStatusBroadcaster
	dataSourceUpdates *DataSourceUpdatesImpl
}

// NewDataSourceStatusProviderImpl creates the internal implementation of DataSourceStatusProvider.
func NewDataSourceStatusProviderImpl(
	broadcaster *DataSourceStatusBroadcaster,
	dataSourceUpdates *DataSourceUpdatesImpl,
) interfaces.DataSourceStatusProvider {
	return &dataSourceStatusProviderImpl{broadcaster, dataSourceUpdates}
}

// GetStatus is a standard method of DataSourceStatusProvider.
func (d *dataSourceStatusProviderImpl) GetStatus() interfaces.DataSourceStatus {
	return d.dataSourceUpdates.GetLastStatus()
}

// AddStatusListener is a standard method of DataSourceStatusProvider.
func (d *dataSourceStatusProviderImpl) AddStatusListener() <-chan interfaces.DataSourceStatus {
	return d.broadcaster.AddListener()
}

// RemoveStatusListener is a standard method of DataSourceStatusProvider.
func (d *dataSourceStatusProviderImpl) RemoveStatusListener(listener <-chan interfaces.DataSourceStatus) {
	d.broadcaster.RemoveListener(listener)
}

// WaitFor is a standard method of DataSourceStatusProvider.
func (d *dataSourceStatusProviderImpl) WaitFor(desiredState interfaces.DataSourceState, timeout time.Duration) bool {
	return d.dataSourceUpdates.waitFor(desiredState, timeout)
}

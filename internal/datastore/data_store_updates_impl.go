package datastore

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal"
)

// DataStoreUpdatesImpl is the internal implementation of DataStoreUpdates. It is exported
// because the actual implementation type, rather than the interface, is required as a dependency
// of other SDK components.
type DataStoreUpdatesImpl struct { //nolint:revive // yes, we know the package name resembles the type name
	lastStatus  interfaces.DataStoreStatus
	broadcaster *internal.Broadcaster[interfaces.DataStoreStatus]
	lock        sync.Mutex
}

// NewDataStoreUpdatesImpl creates the internal implementation of DataStoreUpdates.
func NewDataStoreUpdatesImpl(broadcaster *internal.Broadcaster[interfaces.DataStoreStatus]) *DataStoreUpdatesImpl {
	return &DataStoreUpdatesImpl{
		lastStatus:  interfaces.DataStoreStatus{Available: true},
		broadcaster: broadcaster,
	}
}

func (d *DataStoreUpdatesImpl) getStatus() interfaces.DataStoreStatus {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.lastStatus
}

func (d *DataStoreUpdatesImpl) getBroadcaster() *internal.Broadcaster[interfaces.DataStoreStatus] {
	return d.broadcaster
}

// UpdateStatus is called from the data store to push a status update.
func (d *DataStoreUpdatesImpl) UpdateStatus(newStatus interfaces.DataStoreStatus) {
	d.lock.Lock()
	modified := false
	if newStatus != d.lastStatus {
		d.lastStatus = newStatus
		modified = true
	}
	d.lock.Unlock()
	if modified {
		d.broadcaster.Broadcast(newStatus)
	}
}

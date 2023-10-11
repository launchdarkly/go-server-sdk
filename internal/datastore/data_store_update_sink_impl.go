package datastore

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
)

// DataStoreUpdateSinkImpl is the internal implementation of DataStoreUpdateSink. It is exported
// because the actual implementation type, rather than the interface, is required as a dependency
// of other SDK components.
type DataStoreUpdateSinkImpl struct {
	lastStatus  interfaces.DataStoreStatus
	broadcaster *internal.Broadcaster[interfaces.DataStoreStatus]
	lock        sync.Mutex
}

// NewDataStoreUpdateSinkImpl creates the internal implementation of DataStoreUpdateSink.
func NewDataStoreUpdateSinkImpl(
	broadcaster *internal.Broadcaster[interfaces.DataStoreStatus],
) *DataStoreUpdateSinkImpl {
	return &DataStoreUpdateSinkImpl{
		lastStatus:  interfaces.DataStoreStatus{Available: true},
		broadcaster: broadcaster,
	}
}

func (d *DataStoreUpdateSinkImpl) getStatus() interfaces.DataStoreStatus {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.lastStatus
}

func (d *DataStoreUpdateSinkImpl) getBroadcaster() *internal.Broadcaster[interfaces.DataStoreStatus] {
	return d.broadcaster
}

// UpdateStatus is called from the data store to push a status update.
func (d *DataStoreUpdateSinkImpl) UpdateStatus(newStatus interfaces.DataStoreStatus) {
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

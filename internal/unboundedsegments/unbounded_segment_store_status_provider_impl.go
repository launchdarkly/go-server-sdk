package unboundedsegments

import (
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

// This is the standard implementation of UnboundedSegmentStoreStatusProvider. Most of the work is done by
// UnboundedSegmentStoreManager, which exposes the methods that other SDK components need to access the store.
//
// We always create this component regardless of whether there really is a store. If there is no store (so
// there is no UnboundedSegmentStoreManager) then we won't actually be doing any unbounded segments stuff,
// or sending any status updates, but this API object still exists so your app won't crash if you try to
// use GetStatus or AddStatusListener.
type unboundedSegmentStoreStatusProviderImpl struct {
	manager     *UnboundedSegmentStoreManager
	broadcaster *internal.UnboundedSegmentStoreStatusBroadcaster
}

// NewUnboundedSegmentStoreStatusProviderImpl creates the internal implementation of
// UnboundedSegmentStoreStatusProvider. The manager parameter can be nil if there is no unbounded segment
// store.
func NewUnboundedSegmentStoreStatusProviderImpl(
	manager *UnboundedSegmentStoreManager,
) interfaces.UnboundedSegmentStoreStatusProvider {
	if manager == nil {
		return &unboundedSegmentStoreStatusProviderImpl{
			broadcaster: internal.NewUnboundedSegmentStoreStatusBroadcaster(),
		}
	}
	return &unboundedSegmentStoreStatusProviderImpl{
		manager:     manager,
		broadcaster: manager.getBroadcaster(),
	}
}

func (u *unboundedSegmentStoreStatusProviderImpl) GetStatus() interfaces.UnboundedSegmentStoreStatus {
	if u.manager == nil {
		return interfaces.UnboundedSegmentStoreStatus{Available: false}
	}
	return u.manager.getStatus()
}

func (u *unboundedSegmentStoreStatusProviderImpl) AddStatusListener() <-chan interfaces.UnboundedSegmentStoreStatus {
	return u.broadcaster.AddListener()
}

func (u *unboundedSegmentStoreStatusProviderImpl) RemoveStatusListener(
	ch <-chan interfaces.UnboundedSegmentStoreStatus,
) {
	u.broadcaster.RemoveListener(ch)
}

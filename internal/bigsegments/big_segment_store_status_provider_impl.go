package bigsegments

import (
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

// This is the standard implementation of BigSegmentStoreStatusProvider. Most of the work is done by
// BigSegmentStoreManager, which exposes the methods that other SDK components need to access the store.
//
// We always create this component regardless of whether there really is a store. If there is no store (so
// there is no BigSegmentStoreManager) then we won't actually be doing any big segments stuff, or sending
// any status updates, but this API object still exists so your app won't crash if you try to use
// GetStatus or AddStatusListener.
type bigSegmentStoreStatusProviderImpl struct {
	manager     *BigSegmentStoreManager
	broadcaster *internal.BigSegmentStoreStatusBroadcaster
}

// NewBigSegmentStoreStatusProviderImpl creates the internal implementation of
// BigSegmentStoreStatusProvider. The manager parameter can be nil if there is no big segment store.
func NewBigSegmentStoreStatusProviderImpl(
	manager *BigSegmentStoreManager,
) interfaces.BigSegmentStoreStatusProvider {
	if manager == nil {
		return &bigSegmentStoreStatusProviderImpl{
			broadcaster: internal.NewBigSegmentStoreStatusBroadcaster(),
		}
	}
	return &bigSegmentStoreStatusProviderImpl{
		manager:     manager,
		broadcaster: manager.getBroadcaster(),
	}
}

func (u *bigSegmentStoreStatusProviderImpl) GetStatus() interfaces.BigSegmentStoreStatus {
	if u.manager == nil {
		return interfaces.BigSegmentStoreStatus{Available: false}
	}
	return u.manager.getStatus()
}

func (u *bigSegmentStoreStatusProviderImpl) AddStatusListener() <-chan interfaces.BigSegmentStoreStatus {
	return u.broadcaster.AddListener()
}

func (u *bigSegmentStoreStatusProviderImpl) RemoveStatusListener(
	ch <-chan interfaces.BigSegmentStoreStatus,
) {
	u.broadcaster.RemoveListener(ch)
}

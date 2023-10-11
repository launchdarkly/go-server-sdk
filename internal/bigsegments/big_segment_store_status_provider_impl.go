package bigsegments

import (
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
)

// This is the standard implementation of BigSegmentStoreStatusProvider. Most of the work is done by
// BigSegmentStoreManager, which exposes the methods that other SDK components need to access the store.
//
// We always create this component regardless of whether there really is a store. If there is no store (so
// there is no BigSegmentStoreManager) then we won't actually be doing any Big Segments stuff, or sending
// any status updates, but this API object still exists so your app won't crash if you try to use
// GetStatus or AddStatusListener.
type bigSegmentStoreStatusProviderImpl struct {
	getStatusFn func() interfaces.BigSegmentStoreStatus
	broadcaster *internal.Broadcaster[interfaces.BigSegmentStoreStatus]
}

// NewBigSegmentStoreStatusProviderImpl creates the internal implementation of
// BigSegmentStoreStatusProvider. The manager parameter can be nil if there is no Big Segment store.
func NewBigSegmentStoreStatusProviderImpl(
	getStatusFn func() interfaces.BigSegmentStoreStatus,
	broadcaster *internal.Broadcaster[interfaces.BigSegmentStoreStatus],
) interfaces.BigSegmentStoreStatusProvider {
	return &bigSegmentStoreStatusProviderImpl{
		getStatusFn: getStatusFn,
		broadcaster: broadcaster,
	}
}

func (b *bigSegmentStoreStatusProviderImpl) GetStatus() interfaces.BigSegmentStoreStatus {
	if b.getStatusFn == nil {
		return interfaces.BigSegmentStoreStatus{Available: false}
	}
	return b.getStatusFn()
}

func (b *bigSegmentStoreStatusProviderImpl) AddStatusListener() <-chan interfaces.BigSegmentStoreStatus {
	return b.broadcaster.AddListener()
}

func (b *bigSegmentStoreStatusProviderImpl) RemoveStatusListener(
	ch <-chan interfaces.BigSegmentStoreStatus,
) {
	b.broadcaster.RemoveListener(ch)
}

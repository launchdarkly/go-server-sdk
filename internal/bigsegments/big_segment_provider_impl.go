package bigsegments

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
)

// Internal implementation of the BigSegmentProvider interface from go-server-sdk-evaluation.
// This is a simple wrapper around BigSegmentStoreManager; the latter handles user key hashing
// and caching.

type bigSegmentProviderImpl struct {
	storeManager *BigSegmentStoreManager
}

// NewBigSegmentProviderImpl creates the internal implementation of BigSegmentProvider.
func NewBigSegmentProviderImpl(storeManager *BigSegmentStoreManager) ldeval.BigSegmentProvider {
	return &bigSegmentProviderImpl{
		storeManager: storeManager,
	}
}

// GetUserMembership is called by the evaluator when it needs to get the big segment membership
// state for a user.
func (u *bigSegmentProviderImpl) GetUserMembership(
	userKey string,
) (ldeval.BigSegmentMembership, ldreason.BigSegmentsStatus) {
	membership, ok := u.storeManager.getUserMembership(userKey)
	if !ok {
		return nil, ldreason.BigSegmentsStoreError
	}
	status := ldreason.BigSegmentsHealthy
	if u.storeManager.getStatus().Stale {
		status = ldreason.BigSegmentsStale
	}
	return membership, status
}

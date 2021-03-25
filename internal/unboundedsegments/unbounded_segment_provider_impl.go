package unboundedsegments

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
)

// Internal implementation of the UnboundedSegmentProvider interface from go-server-sdk-evaluation.
// This is a simple wrapper around UnboundedSegmentStoreManager; the latter handles user key hashing
// and caching.

type unboundedSegmentProviderImpl struct {
	storeManager *UnboundedSegmentStoreManager
}

// NewUnboundedSegmentProviderImpl creates the internal implementation of UnboundedSegmentProvider.
func NewUnboundedSegmentProviderImpl(storeManager *UnboundedSegmentStoreManager) ldeval.UnboundedSegmentProvider {
	return &unboundedSegmentProviderImpl{
		storeManager: storeManager,
	}
}

// GetUserMembership is called by the evaluator when it needs to get the unbounded segment membership
// state for a user.
func (u *unboundedSegmentProviderImpl) GetUserMembership(
	userKey string,
) (ldeval.UnboundedSegmentMembership, ldreason.UnboundedSegmentsStatus) {
	membership, ok := u.storeManager.getUserMembership(userKey)
	if !ok {
		return nil, ldreason.UnboundedSegmentsStoreError
	}
	status := ldreason.UnboundedSegmentsHealthy
	if u.storeManager.getStatus().Stale {
		status = ldreason.UnboundedSegmentsStale
	}
	return membership, status
}

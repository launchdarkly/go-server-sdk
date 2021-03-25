package unboundedsegments

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"

	"github.com/stretchr/testify/assert"
)

func TestUnboundedSegmentProviderUserNotFound(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewUnboundedSegmentProviderImpl(p.manager)

		membership, status := provider.GetUserMembership("userkey1")
		assert.Nil(t, membership)
		assert.Equal(t, ldreason.UnboundedSegmentsHealthy, status)
	})
}

func TestUnboundedSegmentProviderUserFoundAndStoreNotStale(t *testing.T) {
	key := "userkey1"
	hash := HashForUserKey(key)
	expectedMembership := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})

	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewUnboundedSegmentProviderImpl(p.manager)
		p.store.TestSetMembership(hash, expectedMembership)

		membership, status := provider.GetUserMembership(key)
		assert.Equal(t, expectedMembership, membership)
		assert.Equal(t, ldreason.UnboundedSegmentsHealthy, status)
	})
}

func TestUnboundedSegmentProviderUserFoundAndStoreStale(t *testing.T) {
	key := "userkey1"
	hash := HashForUserKey(key)
	expectedMembership := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})

	p := storeManagerTest(t)
	p.staleTime = time.Millisecond * 100
	p.run(func(p *storeManagerTestParams) {
		provider := NewUnboundedSegmentProviderImpl(p.manager)
		statusCh := p.manager.getBroadcaster().AddListener()
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: true})
		p.store.TestSetMembership(hash, expectedMembership)

		membership, status := provider.GetUserMembership(key)
		assert.Equal(t, expectedMembership, membership)
		assert.Equal(t, ldreason.UnboundedSegmentsStale, status)
	})
}

func TestUnboundedSegmentProviderStoreError(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewUnboundedSegmentProviderImpl(p.manager)
		fakeError := errors.New("sorry")
		p.store.TestSetMembershipError(fakeError)

		membership, status := provider.GetUserMembership("userkey1")
		assert.Nil(t, membership)
		assert.Equal(t, ldreason.UnboundedSegmentsStoreError, status)
	})
}

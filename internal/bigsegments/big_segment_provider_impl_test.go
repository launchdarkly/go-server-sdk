package bigsegments

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

func TestBigSegmentProviderUserNotFound(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewBigSegmentProviderImpl(p.manager)

		membership, status := provider.GetUserMembership("userkey1")
		assert.Nil(t, membership)
		assert.Equal(t, ldreason.BigSegmentsHealthy, status)
	})
}

func TestBigSegmentProviderUserFoundAndStoreNotStale(t *testing.T) {
	key := "userkey1"
	hash := HashForUserKey(key)
	expectedMembership := ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})

	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewBigSegmentProviderImpl(p.manager)
		p.store.TestSetMembership(hash, expectedMembership)

		membership, status := provider.GetUserMembership(key)
		assert.Equal(t, expectedMembership, membership)
		assert.Equal(t, ldreason.BigSegmentsHealthy, status)
	})
}

func TestBigSegmentProviderUserFoundAndStoreStale(t *testing.T) {
	key := "userkey1"
	hash := HashForUserKey(key)
	expectedMembership := ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})

	p := storeManagerTest(t)
	p.staleTime = time.Millisecond * 100
	p.run(func(p *storeManagerTestParams) {
		provider := NewBigSegmentProviderImpl(p.manager)
		statusCh := p.manager.getBroadcaster().AddListener()
		sharedtest.ExpectBigSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.BigSegmentStoreStatus{Available: true, Stale: false})
		sharedtest.ExpectBigSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.BigSegmentStoreStatus{Available: true, Stale: true})
		p.store.TestSetMembership(hash, expectedMembership)

		membership, status := provider.GetUserMembership(key)
		assert.Equal(t, expectedMembership, membership)
		assert.Equal(t, ldreason.BigSegmentsStale, status)
	})
}

func TestBigSegmentProviderStoreError(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewBigSegmentProviderImpl(p.manager)
		fakeError := errors.New("sorry")
		p.store.TestSetMembershipError(fakeError)

		membership, status := provider.GetUserMembership("userkey1")
		assert.Nil(t, membership)
		assert.Equal(t, ldreason.BigSegmentsStoreError, status)
	})
}

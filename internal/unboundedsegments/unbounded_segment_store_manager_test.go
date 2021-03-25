package unboundedsegments

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"
)

func TestStoreIsQueriedWithHashedUserKey(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		userKey := "userkey"
		userHash := HashForUserKey(userKey)
		expectedMembership := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})
		p.store.TestSetMembership(userHash, expectedMembership)

		p.assertMembership(userKey, expectedMembership)
		p.assertUserHashesQueried(userHash)
	})
}

func TestStoreCachesUser(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		userKey := "userkey"
		userHash := HashForUserKey(userKey)
		expectedMembership := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})
		p.store.TestSetMembership(userHash, expectedMembership)

		p.assertMembership(userKey, expectedMembership)
		p.assertMembership(userKey, expectedMembership)
		p.assertUserHashesQueried(userHash) // only one query was done
	})
}

func TestStoreCachesUserNotFoundResult(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		userKey := "userkey"
		userHash := HashForUserKey(userKey)

		p.assertMembership(userKey, nil)
		p.assertMembership(userKey, nil)
		p.assertUserHashesQueried(userHash) // only one query was done
	})
}

func TestStoreEvictsLeastRecentUserFromCache(t *testing.T) {
	p := storeManagerTest(t)
	p.userCacheSize = 2
	p.run(func(p *storeManagerTestParams) {
		userKey1 := "userkey1"
		userHash1 := HashForUserKey(userKey1)
		expectedMembership1 := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{"yes1"}, []string{"no1"})
		p.store.TestSetMembership(userHash1, expectedMembership1)

		userKey2 := "userkey2"
		userHash2 := HashForUserKey(userKey2)
		expectedMembership2 := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{"yes2"}, []string{"no2"})
		p.store.TestSetMembership(userHash2, expectedMembership2)

		userKey3 := "userkey3"
		userHash3 := HashForUserKey(userKey3)
		expectedMembership3 := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs([]string{"yes3"}, []string{"no3"})
		p.store.TestSetMembership(userHash3, expectedMembership3)

		p.assertMembership(userKey1, expectedMembership1)
		p.assertMembership(userKey2, expectedMembership2)
		p.assertMembership(userKey3, expectedMembership3)

		// Since the capacity is only 2 and userKey1 was the least recently used, that key should be
		// evicted by the userKey3 query. Unfortunately, we have to add a hacky delay here because the
		// LRU behavior of ccache is only eventually consistent - the LRU status is updated by a worker
		// goroutine.
		require.Eventually(t, func() bool {
			return p.manager.userCache.Get(userKey1) == nil
		}, time.Second, time.Millisecond*10, "timed out waiting for LRU eviction")

		p.assertUserHashesQueried(userHash1, userHash2, userHash3)

		p.assertMembership(userKey1, expectedMembership1)

		p.assertUserHashesQueried(userHash1, userHash2, userHash3, userHash1)
	})
}

func TestPollingDetectsAvailabilityChanges(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		statusCh := p.manager.getBroadcaster().AddListener()

		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})

		p.store.TestSetMetadataState(interfaces.UnboundedSegmentStoreMetadata{}, errors.New("sorry"))
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: false, Stale: false})

		p.store.TestSetMetadataToCurrentTime()
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})
	})
}

func TestPollingDetectsStaleStatus(t *testing.T) {
	p := storeManagerTest(t)
	p.staleTime = time.Millisecond * 100
	p.run(func(p *storeManagerTestParams) {
		statusCh := p.manager.getBroadcaster().AddListener()

		stopUpdater := make(chan struct{})
		defer close(stopUpdater)

		var shouldUpdate atomic.Value
		shouldUpdate.Store(true)

		go func() {
			ticker := time.NewTicker(time.Millisecond * 5)
			for {
				select {
				case <-stopUpdater:
					ticker.Stop()
					return
				case <-ticker.C:
					if shouldUpdate.Load() == true {
						p.store.TestSetMetadataToCurrentTime()
					}
				}
			}
		}()

		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})

		shouldUpdate.Store(false)
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Millisecond*200,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: true})

		shouldUpdate.Store(true)
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, p.manager.getStatus, time.Millisecond*200,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})
	})
}

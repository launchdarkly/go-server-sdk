package ldstoreimpl

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/bigsegments"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBigSegmentStoreWrapper(t *testing.T) {
	t.Run("queries store with hashed user key", testBigSegmentStoreWrapperMembershipQuery)
	t.Run("caches membership state", testBigSegmentStoreWrapperMembershipCaching)
	t.Run("sends status updates", testBigSegmentStoreWrapperStatusUpdates)
}

type storeWrapperTestParams struct {
	t             *testing.T
	store         *sharedtest.MockBigSegmentStore
	wrapper       *BigSegmentStoreWrapper
	pollInterval  time.Duration
	staleTime     time.Duration
	userCacheSize int
	userCacheTime time.Duration
	statusCh      chan interfaces.BigSegmentStoreStatus
	mockLog       *ldlogtest.MockLog
}

func storeWrapperTest(t *testing.T) *storeWrapperTestParams {
	return &storeWrapperTestParams{
		t:             t,
		store:         &sharedtest.MockBigSegmentStore{},
		pollInterval:  time.Millisecond * 10,
		staleTime:     time.Hour,
		userCacheSize: 1000,
		userCacheTime: time.Hour,
		statusCh:      make(chan interfaces.BigSegmentStoreStatus, 10),
		mockLog:       ldlogtest.NewMockLog(),
	}
}

func (p *storeWrapperTestParams) run(action func(*storeWrapperTestParams)) {
	defer p.mockLog.DumpIfTestFailed(p.t)
	p.wrapper = NewBigSegmentStoreWrapper(
		p.store,
		func(status interfaces.BigSegmentStoreStatus) { p.statusCh <- status },
		p.pollInterval, p.staleTime, p.userCacheSize, p.userCacheTime, p.mockLog.Loggers,
	)
	p.store.TestSetMetadataToCurrentTime()
	defer p.wrapper.Close()
	action(p)
}

func (p *storeWrapperTestParams) assertMembership(userKey string, expected interfaces.BigSegmentMembership) {
	membership, status := p.wrapper.GetUserMembership(userKey)
	assert.Equal(p.t, ldreason.BigSegmentsHealthy, status)
	assert.Equal(p.t, expected, membership)
}

func (p *storeWrapperTestParams) assertUserHashesQueried(hashes ...string) {
	assert.Equal(p.t, hashes, p.store.TestGetMembershipQueries())
}

func testBigSegmentStoreWrapperMembershipQuery(t *testing.T) {
	storeWrapperTest(t).run(func(p *storeWrapperTestParams) {
		userKey := "userkey"
		userHash := bigsegments.HashForUserKey(userKey)
		expectedMembership := NewBigSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})
		p.store.TestSetMembership(userHash, expectedMembership)

		p.assertMembership(userKey, expectedMembership)
		p.assertUserHashesQueried(userHash)
	})
}

func testBigSegmentStoreWrapperMembershipCaching(t *testing.T) {
	t.Run("successful query is cached", func(t *testing.T) {
		storeWrapperTest(t).run(func(p *storeWrapperTestParams) {
			userKey := "userkey"
			userHash := bigsegments.HashForUserKey(userKey)
			expectedMembership := NewBigSegmentMembershipFromSegmentRefs([]string{"yes"}, []string{"no"})
			p.store.TestSetMembership(userHash, expectedMembership)

			p.assertMembership(userKey, expectedMembership)
			p.assertMembership(userKey, expectedMembership)
			p.assertUserHashesQueried(userHash) // only one query was done
		})
	})

	t.Run("not-found result is cached", func(t *testing.T) {
		storeWrapperTest(t).run(func(p *storeWrapperTestParams) {
			userKey := "userkey"
			userHash := bigsegments.HashForUserKey(userKey)

			p.assertMembership(userKey, nil)
			p.assertMembership(userKey, nil)
			p.assertUserHashesQueried(userHash) // only one query was done
		})
	})

	t.Run("least recent user is evicted from cache", func(t *testing.T) {
		p := storeWrapperTest(t)
		p.userCacheSize = 2
		p.run(func(p *storeWrapperTestParams) {
			userKey1 := "userkey1"
			userHash1 := bigsegments.HashForUserKey(userKey1)
			expectedMembership1 := NewBigSegmentMembershipFromSegmentRefs([]string{"yes1"}, []string{"no1"})
			p.store.TestSetMembership(userHash1, expectedMembership1)

			userKey2 := "userkey2"
			userHash2 := bigsegments.HashForUserKey(userKey2)
			expectedMembership2 := NewBigSegmentMembershipFromSegmentRefs([]string{"yes2"}, []string{"no2"})
			p.store.TestSetMembership(userHash2, expectedMembership2)

			userKey3 := "userkey3"
			userHash3 := bigsegments.HashForUserKey(userKey3)
			expectedMembership3 := NewBigSegmentMembershipFromSegmentRefs([]string{"yes3"}, []string{"no3"})
			p.store.TestSetMembership(userHash3, expectedMembership3)

			p.assertMembership(userKey1, expectedMembership1)
			p.assertMembership(userKey2, expectedMembership2)
			p.assertMembership(userKey3, expectedMembership3)

			// Since the capacity is only 2 and userKey1 was the least recently used, that key should be
			// evicted by the userKey3 query. Unfortunately, we have to add a hacky delay here because the
			// LRU behavior of ccache is only eventually consistent - the LRU status is updated by a worker
			// goroutine.
			require.Eventually(t, func() bool {
				return p.wrapper.userCache.Get(userKey1) == nil
			}, time.Second, time.Millisecond*10, "timed out waiting for LRU eviction")

			p.assertUserHashesQueried(userHash1, userHash2, userHash3)

			p.assertMembership(userKey1, expectedMembership1)

			p.assertUserHashesQueried(userHash1, userHash2, userHash3, userHash1)
		})
	})
}

func testBigSegmentStoreWrapperStatusUpdates(t *testing.T) {
	t.Run("polling detects store unavailability", func(t *testing.T) {
		storeWrapperTest(t).run(func(p *storeWrapperTestParams) {
			sharedtest.ExpectBigSegmentStoreStatus(t, p.statusCh, p.wrapper.GetStatus, time.Second,
				interfaces.BigSegmentStoreStatus{Available: true, Stale: false})

			p.store.TestSetMetadataState(interfaces.BigSegmentStoreMetadata{}, errors.New("sorry"))
			sharedtest.ExpectBigSegmentStoreStatus(t, p.statusCh, p.wrapper.GetStatus, time.Second,
				interfaces.BigSegmentStoreStatus{Available: false, Stale: false})

			p.store.TestSetMetadataToCurrentTime()
			sharedtest.ExpectBigSegmentStoreStatus(t, p.statusCh, p.wrapper.GetStatus, time.Second,
				interfaces.BigSegmentStoreStatus{Available: true, Stale: false})
		})
	})

	t.Run("polling detects stale status", func(t *testing.T) {
		p := storeWrapperTest(t)
		p.staleTime = time.Millisecond * 100
		p.run(func(p *storeWrapperTestParams) {
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

			sharedtest.ExpectBigSegmentStoreStatus(t, p.statusCh, p.wrapper.GetStatus, time.Second,
				interfaces.BigSegmentStoreStatus{Available: true, Stale: false})

			shouldUpdate.Store(false)
			sharedtest.ExpectBigSegmentStoreStatus(t, p.statusCh, p.wrapper.GetStatus, time.Millisecond*200,
				interfaces.BigSegmentStoreStatus{Available: true, Stale: true})

			shouldUpdate.Store(true)
			sharedtest.ExpectBigSegmentStoreStatus(t, p.statusCh, p.wrapper.GetStatus, time.Millisecond*200,
				interfaces.BigSegmentStoreStatus{Available: true, Stale: false})
		})
	})
}

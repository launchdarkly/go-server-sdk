package unboundedsegments

import (
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"

	"github.com/launchdarkly/ccache"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"

	"golang.org/x/sync/singleflight"
)

// UnboundedSegmentStoreManager is the internal component that owns the unbounded segment store, polls its
// status, maintains the user membership cache, and manages status subscriptions.
//
// We only create an instance of this type if there really is a store.
type UnboundedSegmentStoreManager struct {
	store       interfaces.UnboundedSegmentStore
	broadcaster *internal.UnboundedSegmentStoreStatusBroadcaster
	staleTime   time.Duration
	userCache   *ccache.Cache
	cacheTTL    time.Duration
	haveStatus  bool
	lastStatus  interfaces.UnboundedSegmentStoreStatus
	requests    singleflight.Group
	pollCloser  chan struct{}
	loggers     ldlog.Loggers
	lock        sync.RWMutex
}

// NewUnboundedSegmentStoreManager creates the UnboundedSegmentStoreManager. The store must not be nil.
// After this point, the store's lifecycle belongs to the manager, so closing the manager closes the store.
// We also start polling the store at this point.
func NewUnboundedSegmentStoreManager(
	store interfaces.UnboundedSegmentStore,
	pollInterval time.Duration,
	staleTime time.Duration,
	userCacheSize int,
	userCacheTime time.Duration,
	loggers ldlog.Loggers,
) *UnboundedSegmentStoreManager {
	pollCloser := make(chan struct{})
	u := &UnboundedSegmentStoreManager{
		store:       store,
		broadcaster: internal.NewUnboundedSegmentStoreStatusBroadcaster(),
		staleTime:   staleTime,
		userCache:   ccache.New(ccache.Configure().MaxSize(int64(userCacheSize))),
		cacheTTL:    userCacheTime,
		pollCloser:  pollCloser,
		loggers:     loggers,
	}

	go u.runPollTask(pollInterval, pollCloser)

	return u
}

// Close shuts down the manager, the store, the polling task, and the status broadcaster.
func (u *UnboundedSegmentStoreManager) Close() {
	u.lock.Lock()
	if u.pollCloser != nil {
		close(u.pollCloser)
		u.pollCloser = nil
	}
	if u.userCache != nil {
		u.userCache.Stop()
		u.userCache = nil
	}
	u.lock.Unlock()

	u.broadcaster.Close()
	_ = u.store.Close()
}

// getStatus returns an UnboundedSegmentStoreStatus describing whether the store seems to be available
// (that is, the last query to it did not return an error) and whether it is stale (that is, the last
// known update time is too far in the past).
//
// If we have not yet obtained that information (the poll task has not executed yet), then this method
// immediately does a metadata query and waits for it to succeed or fail. This means that if an
// application using unbounded segments evaluates a feature flag immediately after creating the SDK
// client, before the first status poll has happened, that evaluation may block for however long it
// takes to query the store.
func (u *UnboundedSegmentStoreManager) getStatus() interfaces.UnboundedSegmentStoreStatus {
	u.lock.RLock()
	status := u.lastStatus
	haveStatus := u.haveStatus
	u.lock.RUnlock()

	if haveStatus {
		return status
	}

	return u.pollStoreAndUpdateStatus()
}

// getUserMembership either returns a cached UnboundedSegmentMembership for this user key or, if none
// is available, queries and caches the membership for the user after hashing the key. The second
// return value is normally true (even if the user was not found); false indicates a store error or
// other internal error (the caller should not care what the specific error is).
func (u *UnboundedSegmentStoreManager) getUserMembership(userKey string) (interfaces.UnboundedSegmentMembership, bool) {
	entry := u.safeCacheGet(userKey)
	if entry == nil || entry.Expired() {
		// Use singleflight to ensure that we'll only do this query once even if multiple goroutines are
		// requesting it
		value, err, _ := u.requests.Do(userKey, func() (interface{}, error) {
			hash := HashForUserKey(userKey)
			u.loggers.Debugf("querying unbounded segment state for user hash %q", hash)
			return u.store.GetUserMembership(hash)
		})
		if err != nil {
			u.loggers.Errorf("unbounded segment store returned error: %s", err)
			return nil, false
		}
		if value == nil {
			u.safeCacheSet(userKey, nil, u.cacheTTL) // we cache the "not found" status
			return nil, true
		}
		if membership, ok := value.(interfaces.UnboundedSegmentMembership); ok {
			u.safeCacheSet(userKey, membership, u.cacheTTL)
			return membership, true
		}
		u.loggers.Error("UnboundedSegmentStoreManager got wrong value type from request - this should not be possible")
		return nil, false // COVERAGE: can't cause this condition in unit tests
	}
	if entry.Value() == nil { // this is a cached "not found" state
		return nil, true
	}
	if membership, ok := entry.Value().(interfaces.UnboundedSegmentMembership); ok {
		return membership, true
	}
	u.loggers.Error("UnboundedSegmentStoreManager got wrong value type from cache - this should not be possible")
	return nil, false // COVERAGE: can't cause this condition in unit tests
}

func (u *UnboundedSegmentStoreManager) getBroadcaster() *internal.UnboundedSegmentStoreStatusBroadcaster {
	return u.broadcaster
}

func (u *UnboundedSegmentStoreManager) pollStoreAndUpdateStatus() interfaces.UnboundedSegmentStoreStatus {
	var newStatus interfaces.UnboundedSegmentStoreStatus
	u.loggers.Debug("querying unbounded segment store metadata")
	metadata, err := u.store.GetMetadata()

	u.lock.Lock()
	if err == nil {
		newStatus.Available = true
		newStatus.Stale = u.isStale(metadata.LastUpToDate)
		u.loggers.Debugf("unbounded segment store was last updated at %d", metadata.LastUpToDate)
	} else {
		u.loggers.Errorf("unbounded segment store status query returned error: %s", err)
		newStatus.Available = false
	}
	oldStatus := u.lastStatus
	u.lastStatus = newStatus
	hadStatus := u.haveStatus
	u.haveStatus = true
	u.lock.Unlock()

	if !hadStatus || (newStatus != oldStatus) {
		u.loggers.Debugf(
			"unbounded segment store status has changed from %+v to %+v",
			oldStatus,
			newStatus,
		)
		u.broadcaster.Broadcast(newStatus)
	}

	return newStatus
}

func (u *UnboundedSegmentStoreManager) isStale(updateTime ldtime.UnixMillisecondTime) bool {
	age := time.Duration(uint64(ldtime.UnixMillisNow())-uint64(updateTime)) * time.Millisecond
	return age >= u.staleTime
}

func (u *UnboundedSegmentStoreManager) runPollTask(pollInterval time.Duration, pollCloser <-chan struct{}) {
	if pollInterval > u.staleTime {
		pollInterval = u.staleTime // COVERAGE: not really unit-testable due to scheduling indeterminacy
	}
	ticker := time.NewTicker(pollInterval)
	for {
		select {
		case <-pollCloser:
			ticker.Stop()
			return
		case <-ticker.C:
			_ = u.pollStoreAndUpdateStatus()
		}
	}
}

// safeCacheGet and safeCacheSet are necessary because trying to use a ccache.Cache after it's been shut
// down can cause a panic, so we nil it out on Close() and guard it with our lock.
func (u *UnboundedSegmentStoreManager) safeCacheGet(key string) *ccache.Item {
	var ret *ccache.Item
	u.lock.RLock()
	if u.userCache != nil {
		ret = u.userCache.Get(key)
	}
	u.lock.RUnlock()
	return ret
}

func (u *UnboundedSegmentStoreManager) safeCacheSet(key string, value interface{}, ttl time.Duration) {
	u.lock.RLock()
	if u.userCache != nil {
		u.userCache.Set(key, value, ttl)
	}
	u.lock.RUnlock()
}

// HashForUserKey computes the hash that we use in the unbounded segment store. This function is exported
// for use in LDClient tests.
func HashForUserKey(key string) string {
	hashBytes := sha256.Sum256([]byte(key))
	return base64.StdEncoding.EncodeToString(hashBytes[:])
}

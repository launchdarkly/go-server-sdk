package ldstoreimpl

import (
	"sync"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/bigsegments"

	"github.com/launchdarkly/ccache"

	"golang.org/x/sync/singleflight"
)

// BigSegmentStoreWrapper is a component that adds status polling and caching to a BigSegmentStore,
// and provides integration with the evaluation engine.
//
// This component is exposed in the SDK's public API because it needs to be used by the LaunchDarkly
// Relay Proxy as well (or any other component that is calling the evaluation engine in
// go-server-sdk-evaluation directly and needs to provide the same big segment functionality as the
// SDK). It implements the BigSegmentProvider interface that go-server-sdk-evaluation uses to query
// big segments (similar to how ldstoreimpl.NewDataStoreEvaluatorDataProvider provides an
// implementation of the DataProvider interface). It is also responsible for caching membership
// queries and polling the store's metadata to make sure it is not stale.
//
// To avoid unnecessarily exposing implementation details that are subject to change, this type
// should not have any public methods other than GetUserMembership, GetStatus, and Close.
type BigSegmentStoreWrapper struct {
	store          interfaces.BigSegmentStore
	statusUpdateFn func(interfaces.BigSegmentStoreStatus)
	staleTime      time.Duration
	userCache      *ccache.Cache
	cacheTTL       time.Duration
	haveStatus     bool
	lastStatus     interfaces.BigSegmentStoreStatus
	requests       singleflight.Group
	pollCloser     chan struct{}
	loggers        ldlog.Loggers
	lock           sync.RWMutex
}

// NewBigSegmentStoreWrapper creates a BigSegmentStoreWrapper and starts polling the store's status.
//
// The BigSegmentStoreWrapper takes ownership of the BigSegmentStore's lifecycle at this point, so
// calling Close on the BigSegmentStoreWrapper will also close the store.
//
// If not nil, statusUpdateFn will be called whenever the store status has changed.
func NewBigSegmentStoreWrapper(
	store interfaces.BigSegmentStore,
	statusUpdateFn func(interfaces.BigSegmentStoreStatus),
	pollInterval time.Duration,
	staleTime time.Duration,
	userCacheSize int,
	userCacheTime time.Duration,
	loggers ldlog.Loggers,
) *BigSegmentStoreWrapper {
	pollCloser := make(chan struct{})
	w := &BigSegmentStoreWrapper{
		store:          store,
		statusUpdateFn: statusUpdateFn,
		staleTime:      staleTime,
		userCache:      ccache.New(ccache.Configure().MaxSize(int64(userCacheSize))),
		cacheTTL:       userCacheTime,
		pollCloser:     pollCloser,
		loggers:        loggers,
	}

	go w.runPollTask(pollInterval, pollCloser)

	return w
}

// Close shuts down the manager, the store, and the polling task.
func (w *BigSegmentStoreWrapper) Close() {
	w.lock.Lock()
	if w.pollCloser != nil {
		close(w.pollCloser)
		w.pollCloser = nil
	}
	if w.userCache != nil {
		w.userCache.Stop()
		w.userCache = nil
	}
	w.lock.Unlock()

	_ = w.store.Close()
}

// GetUserMembership is called by the evaluator when it needs to get the big segment membership
// state for a user.
//
// If there is a cached membership state for the user, it returns the cached state. Otherwise,
// it converts the user key into the hash string used by the BigSegmentStore, queries the store,
// and caches the result. The returned status value indicates whether the query succeeded, and
// whether the result (regardless of whether it was from a new query or the cache) should be
// considered "stale".
func (w *BigSegmentStoreWrapper) GetUserMembership(
	userKey string,
) (ldeval.BigSegmentMembership, ldreason.BigSegmentsStatus) {
	entry := w.safeCacheGet(userKey)
	var result ldeval.BigSegmentMembership
	if entry == nil || entry.Expired() {
		// Use singleflight to ensure that we'll only do this query once even if multiple goroutines are
		// requesting it
		value, err, _ := w.requests.Do(userKey, func() (interface{}, error) {
			hash := bigsegments.HashForUserKey(userKey)
			w.loggers.Debugf("querying big segment state for user hash %q", hash)
			return w.store.GetUserMembership(hash)
		})
		if err != nil {
			w.loggers.Errorf("big segment store returned error: %s", err)
			return nil, ldreason.BigSegmentsStoreError
		}
		if value == nil {
			w.safeCacheSet(userKey, nil, w.cacheTTL) // we cache the "not found" status
			return nil, ldreason.BigSegmentsHealthy
		}
		if membership, ok := value.(interfaces.BigSegmentMembership); ok {
			w.safeCacheSet(userKey, membership, w.cacheTTL)
			result = membership
		} else {
			w.loggers.Error("BigSegmentStoreWrapper got wrong value type from request - this should not be possible")
			return nil, ldreason.BigSegmentsStoreError
		}
	} else if entry.Value() != nil { // nil is a cached "not found" state
		if membership, ok := entry.Value().(interfaces.BigSegmentMembership); ok {
			result = membership
		} else {
			w.loggers.Error("BigSegmentStoreWrapper got wrong value type from cache - this should not be possible")
			return nil, ldreason.BigSegmentsStoreError // COVERAGE: can't cause this condition in unit tests
		}
	}

	status := ldreason.BigSegmentsHealthy
	if w.GetStatus().Stale {
		status = ldreason.BigSegmentsStale
	}

	return result, status
}

// GetStatus returns a BigSegmentStoreStatus describing whether the store seems to be available
// (that is, the last query to it did not return an error) and whether it is stale (that is, the last
// known update time is too far in the past).
//
// If we have not yet obtained that information (the poll task has not executed yet), then this method
// immediately does a metadata query and waits for it to succeed or fail. This means that if an
// application using big segments evaluates a feature flag immediately after creating the SDK
// client, before the first status poll has happened, that evaluation may block for however long it
// takes to query the store.
func (w *BigSegmentStoreWrapper) GetStatus() interfaces.BigSegmentStoreStatus {
	w.lock.RLock()
	status := w.lastStatus
	haveStatus := w.haveStatus
	w.lock.RUnlock()

	if haveStatus {
		return status
	}

	return w.pollStoreAndUpdateStatus()
}

func (w *BigSegmentStoreWrapper) pollStoreAndUpdateStatus() interfaces.BigSegmentStoreStatus {
	var newStatus interfaces.BigSegmentStoreStatus
	w.loggers.Debug("querying big segment store metadata")
	metadata, err := w.store.GetMetadata()

	w.lock.Lock()
	if err == nil {
		newStatus.Available = true
		newStatus.Stale = w.isStale(metadata.LastUpToDate)
		w.loggers.Debugf("big segment store was last updated at %d", metadata.LastUpToDate)
	} else {
		w.loggers.Errorf("big segment store status query returned error: %s", err)
		newStatus.Available = false
	}
	oldStatus := w.lastStatus
	w.lastStatus = newStatus
	hadStatus := w.haveStatus
	w.haveStatus = true
	w.lock.Unlock()

	if !hadStatus || (newStatus != oldStatus) {
		w.loggers.Debugf(
			"big segment store status has changed from %+v to %+v",
			oldStatus,
			newStatus,
		)
		if w.statusUpdateFn != nil {
			w.statusUpdateFn(newStatus)
		}
	}

	return newStatus
}

func (w *BigSegmentStoreWrapper) isStale(updateTime ldtime.UnixMillisecondTime) bool {
	age := time.Duration(uint64(ldtime.UnixMillisNow())-uint64(updateTime)) * time.Millisecond
	return age >= w.staleTime
}

func (w *BigSegmentStoreWrapper) runPollTask(pollInterval time.Duration, pollCloser <-chan struct{}) {
	if pollInterval > w.staleTime {
		pollInterval = w.staleTime // COVERAGE: not really unit-testable due to scheduling indeterminacy
	}
	ticker := time.NewTicker(pollInterval)
	for {
		select {
		case <-pollCloser:
			ticker.Stop()
			return
		case <-ticker.C:
			_ = w.pollStoreAndUpdateStatus()
		}
	}
}

// safeCacheGet and safeCacheSet are necessary because trying to use a ccache.Cache after it's been shut
// down can cause a panic, so we nil it out on Close() and guard it with our lock.
func (w *BigSegmentStoreWrapper) safeCacheGet(key string) *ccache.Item {
	var ret *ccache.Item
	w.lock.RLock()
	if w.userCache != nil {
		ret = w.userCache.Get(key)
	}
	w.lock.RUnlock()
	return ret
}

func (w *BigSegmentStoreWrapper) safeCacheSet(key string, value interface{}, ttl time.Duration) {
	w.lock.RLock()
	if w.userCache != nil {
		w.userCache.Set(key, value, ttl)
	}
	w.lock.RUnlock()
}

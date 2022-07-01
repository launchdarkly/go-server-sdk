package ldstoreimpl

import (
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	ldeval "github.com/launchdarkly/go-server-sdk-evaluation/v2"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/bigsegments"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"

	"github.com/launchdarkly/ccache"

	"golang.org/x/sync/singleflight"
)

// BigSegmentStoreWrapper is a component that adds status polling and caching to a BigSegmentStore,
// and provides integration with the evaluation engine.
//
// This component is exposed in the SDK's public API because it needs to be used by the LaunchDarkly
// Relay Proxy as well (or any other component that is calling the evaluation engine in
// go-server-sdk-evaluation directly and needs to provide the same Big Segment functionality as the
// SDK). It implements the BigSegmentProvider interface that go-server-sdk-evaluation uses to query
// Big Segments (similar to how ldstoreimpl.NewDataStoreEvaluatorDataProvider provides an
// implementation of the DataProvider interface). It is also responsible for caching membership
// queries and polling the store's metadata to make sure it is not stale.
//
// To avoid unnecessarily exposing implementation details that are subject to change, this type
// should not have any public methods that are not strictly necessary for its use by the SDK and by
// the Relay Proxy.
type BigSegmentStoreWrapper struct {
	store          subsystems.BigSegmentStore
	statusUpdateFn func(interfaces.BigSegmentStoreStatus)
	staleTime      time.Duration
	contextCache   *ccache.Cache
	cacheTTL       time.Duration
	pollInterval   time.Duration
	haveStatus     bool
	lastStatus     interfaces.BigSegmentStoreStatus
	requests       singleflight.Group
	pollCloser     chan struct{}
	pollingActive  bool
	loggers        ldlog.Loggers
	lock           sync.RWMutex
}

// NewBigSegmentStoreWrapperWithConfig creates a BigSegmentStoreWrapper.
//
// It also immediately begins polling the store status, unless config.StatusPollingInitiallyPaused
// is true.
//
// The BigSegmentStoreWrapper takes ownership of the BigSegmentStore's lifecycle at this point, so
// calling Close on the BigSegmentStoreWrapper will also close the store.
//
// If not nil, statusUpdateFn will be called whenever the store status has changed.
func NewBigSegmentStoreWrapperWithConfig(
	config BigSegmentsConfigurationProperties,
	statusUpdateFn func(interfaces.BigSegmentStoreStatus),
	loggers ldlog.Loggers,
) *BigSegmentStoreWrapper {
	pollCloser := make(chan struct{})
	w := &BigSegmentStoreWrapper{
		store:          config.Store,
		statusUpdateFn: statusUpdateFn,
		staleTime:      config.StaleAfter,
		contextCache:   ccache.New(ccache.Configure().MaxSize(int64(config.ContextCacheSize))),
		cacheTTL:       config.ContextCacheTime,
		pollInterval:   config.StatusPollInterval,
		pollCloser:     pollCloser,
		pollingActive:  config.StartPolling,
		loggers:        loggers,
	}

	if config.StartPolling {
		go w.runPollTask(config.StatusPollInterval, pollCloser)
	}

	return w
}

// Close shuts down the manager, the store, and the polling task.
func (w *BigSegmentStoreWrapper) Close() {
	w.lock.Lock()
	if w.pollCloser != nil {
		close(w.pollCloser)
		w.pollCloser = nil
	}
	if w.contextCache != nil {
		w.contextCache.Stop()
		w.contextCache = nil
	}
	w.lock.Unlock()

	_ = w.store.Close()
}

// GetMembership is called by the evaluator when it needs to get the Big Segment membership
// state for an evaluation context.
//
// If there is a cached membership state for the context, it returns the cached state. Otherwise,
// it converts the context key into the hash string used by the BigSegmentStore, queries the store,
// and caches the result. The returned status value indicates whether the query succeeded, and
// whether the result (regardless of whether it was from a new query or the cache) should be
// considered "stale".
//
// We do not need to know the context kind, because each big segment can only be for one kind.
// Thus, if the memberships for context key "x" include segments A and B, it is OK if segment A
// is referring to the context {"kind": "user", "key": x"} while segment B is referring to the
// context {"kind": "org", "key": "x"}; even though those are two different contexts, there is
// no ambiguity when it comes to checking against either of those segments.
func (w *BigSegmentStoreWrapper) GetMembership(
	contextKey string,
) (ldeval.BigSegmentMembership, ldreason.BigSegmentsStatus) {
	entry := w.safeCacheGet(contextKey)
	var result ldeval.BigSegmentMembership
	if entry == nil || entry.Expired() {
		// Use singleflight to ensure that we'll only do this query once even if multiple goroutines are
		// requesting it
		value, err, _ := w.requests.Do(contextKey, func() (interface{}, error) {
			hash := bigsegments.HashForContextKey(contextKey)
			w.loggers.Debugf("querying Big Segment state for context hash %q", hash)
			return w.store.GetMembership(hash)
		})
		if err != nil {
			w.loggers.Errorf("Big Segment store returned error: %s", err)
			return nil, ldreason.BigSegmentsStoreError
		}
		if value == nil {
			w.safeCacheSet(contextKey, nil, w.cacheTTL) // we cache the "not found" status
			return nil, ldreason.BigSegmentsHealthy
		}
		if membership, ok := value.(subsystems.BigSegmentMembership); ok {
			w.safeCacheSet(contextKey, membership, w.cacheTTL)
			result = membership
		} else {
			w.loggers.Error("BigSegmentStoreWrapper got wrong value type from request - this should not be possible")
			return nil, ldreason.BigSegmentsStoreError
		}
	} else if entry.Value() != nil { // nil is a cached "not found" state
		if membership, ok := entry.Value().(subsystems.BigSegmentMembership); ok {
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
// application using Big Segments evaluates a feature flag immediately after creating the SDK
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

// ClearCache invalidates the cache of per-context Big Segment state, so subsequent queries will get
// the latest data.
//
// This is used by the Relay Proxy, but is not currently used by the SDK otherwise.
func (w *BigSegmentStoreWrapper) ClearCache() {
	w.lock.Lock()
	if w.contextCache != nil {
		w.contextCache.Clear()
	}
	w.lock.Unlock()
	w.loggers.Debug("invalidated cache")
}

// SetPollingActive switches the polling task on or off.
//
// This is used by the Relay Proxy, but is not currently used by the SDK otherwise.
func (w *BigSegmentStoreWrapper) SetPollingActive(active bool) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.pollingActive != active {
		w.pollingActive = active
		if active {
			w.pollCloser = make(chan struct{})
			go w.runPollTask(w.pollInterval, w.pollCloser)
		} else if w.pollCloser != nil {
			close(w.pollCloser)
			w.pollCloser = nil
		}
		w.loggers.Debugf("setting status polling to %t", active)
	}
}

func (w *BigSegmentStoreWrapper) pollStoreAndUpdateStatus() interfaces.BigSegmentStoreStatus {
	var newStatus interfaces.BigSegmentStoreStatus
	w.loggers.Debug("querying Big Segment store metadata")
	metadata, err := w.store.GetMetadata()

	w.lock.Lock()
	if err == nil {
		newStatus.Available = true
		newStatus.Stale = w.isStale(metadata.LastUpToDate)
		w.loggers.Debugf("Big Segment store was last updated at %d", metadata.LastUpToDate)
	} else {
		w.loggers.Errorf("Big Segment store status query returned error: %s", err)
		newStatus.Available = false
	}
	oldStatus := w.lastStatus
	w.lastStatus = newStatus
	hadStatus := w.haveStatus
	w.haveStatus = true
	w.lock.Unlock()

	if !hadStatus || (newStatus != oldStatus) {
		w.loggers.Debugf(
			"Big Segment store status has changed from %+v to %+v",
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
	if w.contextCache != nil {
		ret = w.contextCache.Get(key)
	}
	w.lock.RUnlock()
	return ret
}

func (w *BigSegmentStoreWrapper) safeCacheSet(key string, value interface{}, ttl time.Duration) {
	w.lock.RLock()
	if w.contextCache != nil {
		w.contextCache.Set(key, value, ttl)
	}
	w.lock.RUnlock()
}

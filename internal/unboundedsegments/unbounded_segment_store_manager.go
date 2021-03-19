package unboundedsegments

import (
	"sync"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

// UnboundedSegmentStoreManager is the internal component that owns the unbounded segment store, polls its
// status, and manages status subscriptions.
//
// We only create an instance of this type if there really is a store.
type UnboundedSegmentStoreManager struct {
	store        interfaces.UnboundedSegmentStore
	broadcaster  *internal.UnboundedSegmentStoreStatusBroadcaster
	pollInterval time.Duration
	staleTime    time.Duration
	haveStatus   bool
	lastStatus   interfaces.UnboundedSegmentStoreStatus
	pollCloser   chan struct{}
	lock         sync.RWMutex
}

// NewUnboundedSegmentStoreManager creates the UnboundedSegmentStoreManager. The store must not be nil.
// After this point, the store's lifecycle belongs to the manager, so closing the manager closes the store.
// We also start polling the store at this point.
func NewUnboundedSegmentStoreManager(
	store interfaces.UnboundedSegmentStore,
	pollInterval time.Duration,
	staleTime time.Duration,
) *UnboundedSegmentStoreManager {
	u := &UnboundedSegmentStoreManager{
		store:        store,
		broadcaster:  internal.NewUnboundedSegmentStoreStatusBroadcaster(),
		pollInterval: pollInterval,
		staleTime:    staleTime,
		pollCloser:   make(chan struct{}),
	}

	go func() {
		pollInterval := u.pollInterval
		if pollInterval > u.staleTime {
			pollInterval = u.staleTime
		}
		ticker := time.NewTicker(pollInterval)
		for {
			select {
			case <-u.pollCloser:
				ticker.Stop()
				return
			case <-ticker.C:
				_ = u.pollStoreAndUpdateStatus()
			}
		}
	}()

	return u
}

// Close shuts down the manager, the store, the polling task, and the status broadcaster.
func (u *UnboundedSegmentStoreManager) Close() {
	close(u.pollCloser)
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

func (u *UnboundedSegmentStoreManager) getBroadcaster() *internal.UnboundedSegmentStoreStatusBroadcaster {
	return u.broadcaster
}

func (u *UnboundedSegmentStoreManager) pollStoreAndUpdateStatus() interfaces.UnboundedSegmentStoreStatus {
	var newStatus interfaces.UnboundedSegmentStoreStatus
	metadata, err := u.store.GetMetadata()

	u.lock.Lock()
	if err == nil {
		newStatus.Available = true
		newStatus.Stale = u.isStale(metadata.LastUpToDate)
	} else {
		newStatus.Available = false
	}
	oldStatus := u.lastStatus
	u.lastStatus = newStatus
	hadStatus := u.haveStatus
	u.haveStatus = true
	u.lock.Unlock()

	if !hadStatus || (newStatus != oldStatus) {
		u.broadcaster.Broadcast(newStatus)
	}

	return newStatus
}

func (u *UnboundedSegmentStoreManager) isStale(updateTime ldtime.UnixMillisecondTime) bool {
	age := time.Duration(uint64(ldtime.UnixMillisNow())-uint64(updateTime)) * time.Millisecond
	return age >= u.staleTime
}

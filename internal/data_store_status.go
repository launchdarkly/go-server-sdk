package internal

import (
	"sync"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

// DataStoreStatus is a description of whether a data store is functioning normally.
type DataStoreStatus struct {
	// True if the store is currently usable. For a persistent store, this will be false if the last
	// database operation failed and we have not yet seen evidence that the database is working.
	Available bool
	// True if the store may be out of date due to a previous outage, so the SDK should attempt to
	// refresh all feature flag data and rewrite it to the store.
	NeedsRefresh bool
}

// DataStoreStatusProvider is an optional interface that can be implemented by a DataStore.
// It allows other SDK components to detect whether the store is in a usable state.
type DataStoreStatusProvider interface {
	// GetStoreStatus returns the current status of the store.
	GetStoreStatus() DataStoreStatus
	// StatusSubscribe creates a channel that will receive all changes in store status.
	StatusSubscribe() DataStoreStatusSubscription
}

// DataStoreStatusSubscription represents a subscription to data store status updates.
type DataStoreStatusSubscription interface {
	// The channel for receiving updates.
	Channel() <-chan DataStoreStatus
	// Stops the subscription, closing the channel.
	Close()
}

type dataStoreStatusSubcription struct {
	ch    chan DataStoreStatus
	owner *DataStoreStatusManager
}

// DataStoreStatusManager manages status subscriptions and can poll for recovery.
type DataStoreStatusManager struct {
	subs              []chan DataStoreStatus
	lock              sync.Mutex
	lastAvailable     bool
	pollFn            func() bool
	refreshOnRecovery bool
	pollCloser        chan struct{}
	closeOnce         sync.Once
	loggers           ldlog.Loggers
}

var statusPollInterval = time.Millisecond * 500

// NewDataStoreStatusManager creates a new DataStoreStatusManager. The pollFn should return
// true if the store is available, false if not.
func NewDataStoreStatusManager(availableNow bool, pollFn func() bool, refreshOnRecovery bool,
	loggers ldlog.Loggers) *DataStoreStatusManager {
	return &DataStoreStatusManager{
		lastAvailable:     availableNow,
		pollFn:            pollFn,
		refreshOnRecovery: refreshOnRecovery,
		loggers:           loggers,
	}
}

// Subscribe opens a channel for status updates.
func (m *DataStoreStatusManager) Subscribe() DataStoreStatusSubscription {
	ch := make(chan DataStoreStatus, 10)
	sub := &dataStoreStatusSubcription{ch: ch, owner: m}
	m.lock.Lock()
	defer m.lock.Unlock()
	m.subs = append(m.subs, ch)
	return sub
}

func (m *DataStoreStatusManager) unsubscribe(subCh chan DataStoreStatus) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for i, ch := range m.subs {
		if subCh == ch {
			m.subs = append(m.subs[:i], m.subs[i+1:]...)
			break
		}
	}
	close(subCh)
}

// UpdateAvailability signals that the store is now available or unavailable. If that is a change,
// an update will be sent (and, if the new status is unavailable, it will start polling for recovery).
func (m *DataStoreStatusManager) UpdateAvailability(available bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if available == m.lastAvailable {
		return
	}
	m.lastAvailable = available
	newStatus := DataStoreStatus{Available: available}
	if available {
		m.loggers.Warn("Persistent store is available again")
		newStatus.NeedsRefresh = m.refreshOnRecovery
	}

	// Notify all the subscribers (on another goroutine, to make sure we can't be blocked by a
	// slow consumer).
	subs := make([]chan DataStoreStatus, len(m.subs))
	copy(subs, m.subs)

	// We'll dispatch these on another goroutine to make sure we can't be blocked by a slow consumer.
	go func() {
		for _, ch := range subs {
			ch <- newStatus
		}
	}()

	// If the store has just become unavailable, start a poller to detect when it comes back.
	if !available {
		m.loggers.Warn("Detected persistent store unavailability; updates will be cached until it recovers")
		// Start a goroutine to poll until the store starts working again or we shut down.
		m.pollCloser = m.startStatusPoller()
	}
}

// IsAvailable tests whether the last known status was available.
func (m *DataStoreStatusManager) IsAvailable() bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.lastAvailable
}

// Close shuts down all channels and goroutines used by the manager.
func (m *DataStoreStatusManager) Close() {
	m.closeOnce.Do(func() {
		if m.pollCloser != nil {
			close(m.pollCloser)
			m.pollCloser = nil
		}
		for _, s := range m.subs {
			close(s)
		}
	})
}

func (m *DataStoreStatusManager) startStatusPoller() chan struct{} {
	closer := make(chan struct{})
	go func() {
		ticker := time.NewTicker(statusPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if m.pollFn() {
					m.UpdateAvailability(true)
					return
				}
			case <-closer:
				return
			}
		}
	}()
	return closer
}

func (s *dataStoreStatusSubcription) Channel() <-chan DataStoreStatus {
	return s.ch
}

func (s *dataStoreStatusSubcription) Close() {
	s.owner.unsubscribe(s.ch)
}

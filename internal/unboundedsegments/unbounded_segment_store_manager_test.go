package unboundedsegments

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

func TestPollingDetectsAvailabilityChanges(t *testing.T) {
	store := &sharedtest.MockUnboundedSegmentStore{}
	store.SetMetadataToCurrentTime()

	impl := NewUnboundedSegmentStoreManager(store, time.Millisecond*10, time.Second)
	defer impl.Close()

	statusCh := impl.getBroadcaster().AddListener()

	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, impl.getStatus, time.Second,
		interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})

	store.SetMetadataState(interfaces.UnboundedSegmentStoreMetadata{}, errors.New("sorry"))
	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, impl.getStatus, time.Second,
		interfaces.UnboundedSegmentStoreStatus{Available: false, Stale: false})

	store.SetMetadataToCurrentTime()
	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, impl.getStatus, time.Second,
		interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})
}

func TestPollingDetectsStaleStatus(t *testing.T) {
	store := &sharedtest.MockUnboundedSegmentStore{}
	store.SetMetadataToCurrentTime()

	impl := NewUnboundedSegmentStoreManager(store, time.Millisecond*10, time.Millisecond*100)
	defer impl.Close()

	statusCh := impl.getBroadcaster().AddListener()

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
					store.SetMetadataToCurrentTime()
				}
			}
		}
	}()

	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, impl.getStatus, time.Second,
		interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})

	shouldUpdate.Store(false)
	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, impl.getStatus, time.Millisecond*200,
		interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: true})

	shouldUpdate.Store(true)
	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, impl.getStatus, time.Millisecond*200,
		interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})
}

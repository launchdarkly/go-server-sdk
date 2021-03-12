package unboundedsegments

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
)

func TestGetStatus(t *testing.T) {
	store := &sharedtest.MockUnboundedSegmentStore{}
	store.SetMetadataToCurrentTime()

	manager := NewUnboundedSegmentStoreManager(store, time.Second, time.Second)
	defer manager.Close()
	provider := NewUnboundedSegmentStoreStatusProviderImpl(manager)

	status := provider.GetStatus()
	assert.True(t, status.Available)
	assert.False(t, status.Stale)
}

func TestGetStatusWhenNoStoreExists(t *testing.T) {
	provider := NewUnboundedSegmentStoreStatusProviderImpl(nil)

	status := provider.GetStatus()
	assert.False(t, status.Available)
	assert.False(t, status.Stale)
}

func TestStatusListener(t *testing.T) {
	store := &sharedtest.MockUnboundedSegmentStore{}
	store.SetMetadataToCurrentTime()

	manager := NewUnboundedSegmentStoreManager(store, time.Millisecond*10, time.Second)
	defer manager.Close()
	provider := NewUnboundedSegmentStoreStatusProviderImpl(manager)

	statusCh := provider.AddStatusListener()

	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
		interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})

	store.SetMetadataState(interfaces.UnboundedSegmentStoreMetadata{}, errors.New("sorry"))
	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
		interfaces.UnboundedSegmentStoreStatus{Available: false, Stale: false})

	store.SetMetadataToCurrentTime()
	sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
		interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})
}

func TestStatusListenerWhenNoStoreExists(t *testing.T) {
	provider := NewUnboundedSegmentStoreStatusProviderImpl(nil)

	statusCh := provider.AddStatusListener()
	assert.NotNil(t, statusCh) // nothing will be sent on this channel, but there should be one
}

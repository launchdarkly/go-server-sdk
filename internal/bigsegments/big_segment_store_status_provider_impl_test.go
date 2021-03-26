package bigsegments

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
)

func TestGetStatusWhenNoStoreExists(t *testing.T) {
	provider := NewBigSegmentStoreStatusProviderImpl(nil)

	status := provider.GetStatus()
	assert.False(t, status.Available)
	assert.False(t, status.Stale)
}

func TestStatusListener(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewBigSegmentStoreStatusProviderImpl(p.manager)
		p.store.TestSetMetadataToCurrentTime()

		statusCh := provider.AddStatusListener()

		sharedtest.ExpectBigSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
			interfaces.BigSegmentStoreStatus{Available: true, Stale: false})

		p.store.TestSetMetadataState(interfaces.BigSegmentStoreMetadata{}, errors.New("sorry"))
		sharedtest.ExpectBigSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
			interfaces.BigSegmentStoreStatus{Available: false, Stale: false})

		p.store.TestSetMetadataToCurrentTime()
		sharedtest.ExpectBigSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
			interfaces.BigSegmentStoreStatus{Available: true, Stale: false})
	})
}

func TestStatusListenerWhenNoStoreExists(t *testing.T) {
	provider := NewBigSegmentStoreStatusProviderImpl(nil)

	statusCh := provider.AddStatusListener()
	assert.NotNil(t, statusCh) // nothing will be sent on this channel, but there should be one
}

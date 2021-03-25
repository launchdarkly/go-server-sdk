package unboundedsegments

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
)

func TestGetStatusWhenNoStoreExists(t *testing.T) {
	provider := NewUnboundedSegmentStoreStatusProviderImpl(nil)

	status := provider.GetStatus()
	assert.False(t, status.Available)
	assert.False(t, status.Stale)
}

func TestStatusListener(t *testing.T) {
	storeManagerTest(t).run(func(p *storeManagerTestParams) {
		provider := NewUnboundedSegmentStoreStatusProviderImpl(p.manager)
		p.store.TestSetMetadataToCurrentTime()

		statusCh := provider.AddStatusListener()

		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})

		p.store.TestSetMetadataState(interfaces.UnboundedSegmentStoreMetadata{}, errors.New("sorry"))
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: false, Stale: false})

		p.store.TestSetMetadataToCurrentTime()
		sharedtest.ExpectUnboundedSegmentStoreStatus(t, statusCh, provider.GetStatus, time.Second,
			interfaces.UnboundedSegmentStoreStatus{Available: true, Stale: false})
	})
}

func TestStatusListenerWhenNoStoreExists(t *testing.T) {
	provider := NewUnboundedSegmentStoreStatusProviderImpl(nil)

	statusCh := provider.AddStatusListener()
	assert.NotNil(t, statusCh) // nothing will be sent on this channel, but there should be one
}

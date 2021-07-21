package bigsegments

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
)

func TestGetStatusWhenStatusFunctionIsUndefined(t *testing.T) {
	provider := NewBigSegmentStoreStatusProviderImpl(nil, nil)

	status := provider.GetStatus()
	assert.False(t, status.Available)
	assert.False(t, status.Stale)
}

func TestStatusListener(t *testing.T) {
	broadcaster := internal.NewBigSegmentStoreStatusBroadcaster()
	defer broadcaster.Close()
	provider := NewBigSegmentStoreStatusProviderImpl(nil, broadcaster)

	ch1 := provider.AddStatusListener()
	ch2 := provider.AddStatusListener()
	ch3 := provider.AddStatusListener()
	provider.RemoveStatusListener(ch2)

	status := interfaces.BigSegmentStoreStatus{Available: false, Stale: false}
	broadcaster.Broadcast(status)
	sharedtest.ExpectBigSegmentStoreStatus(t, ch1, nil, time.Second, status)
	sharedtest.ExpectBigSegmentStoreStatus(t, ch3, nil, time.Second, status)
	assert.Len(t, ch2, 0)
}

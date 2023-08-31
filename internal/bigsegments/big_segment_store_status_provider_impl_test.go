package bigsegments

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/stretchr/testify/assert"
)

func TestGetStatusWhenStatusFunctionIsUndefined(t *testing.T) {
	provider := NewBigSegmentStoreStatusProviderImpl(nil, nil)

	status := provider.GetStatus()
	assert.False(t, status.Available)
	assert.False(t, status.Stale)
}

func TestStatusListener(t *testing.T) {
	broadcaster := internal.NewBroadcaster[interfaces.BigSegmentStoreStatus]()
	defer broadcaster.Close()
	provider := NewBigSegmentStoreStatusProviderImpl(nil, broadcaster)

	ch1 := provider.AddStatusListener()
	ch2 := provider.AddStatusListener()
	ch3 := provider.AddStatusListener()
	provider.RemoveStatusListener(ch2)

	status := interfaces.BigSegmentStoreStatus{Available: false, Stale: false}
	broadcaster.Broadcast(status)
	mocks.ExpectBigSegmentStoreStatus(t, ch1, nil, time.Second, status)
	mocks.ExpectBigSegmentStoreStatus(t, ch3, nil, time.Second, status)
	assert.Len(t, ch2, 0)
}

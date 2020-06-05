package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func withMockDataSourceUpdates(action func(*sharedtest.MockDataSourceUpdates)) {
	d := sharedtest.NewMockDataSourceUpdates(internal.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	// currently don't need to defer any cleanup actions
	action(d)
}

func waitForReadyWithTimeout(t *testing.T, closeWhenReady <-chan struct{}, timeout time.Duration) {
	select {
	case <-closeWhenReady:
		return
	case <-time.After(timeout):
		require.Fail(t, "timed out waiting for data source to finish starting")
	}
}

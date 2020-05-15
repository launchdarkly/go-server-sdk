package ldcomponents

import (
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func withMockDataSourceUpdates(action func(*sharedtest.MockDataSourceUpdates)) {
	d := sharedtest.NewMockDataSourceUpdates(internal.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	// currently don't need to defer any cleanup actions
	action(d)
}

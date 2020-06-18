package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func TestExternalUpdatesOnly(t *testing.T) {
	dsu := sharedtest.NewMockDataSourceUpdates(internal.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	ds, err := ExternalUpdatesOnly().CreateDataSource(basicClientContext(), dsu)
	require.NoError(t, err)
	defer ds.Close()

	assert.Equal(t, internal.NewNullDataSource(), ds)
	assert.True(t, ds.IsInitialized())

	dsu.RequireStatusOf(t, interfaces.DataSourceStateValid)
}

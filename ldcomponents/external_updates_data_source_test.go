package ldcomponents

import (
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

func TestExternalUpdatesOnly(t *testing.T) {
	dsu := mocks.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	context := subsystems.BasicClientContext{DataSourceUpdateSink: dsu}
	ds, err := ExternalUpdatesOnly().Build(context)
	require.NoError(t, err)
	defer ds.Close()

	assert.Equal(t, datasource.NewNullDataSource(), ds)
	assert.True(t, ds.IsInitialized())

	dsu.RequireStatusOf(t, interfaces.DataSourceStateValid)
}

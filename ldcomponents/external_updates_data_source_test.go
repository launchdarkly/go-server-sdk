package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
)

func TestExternalUpdatesOnly(t *testing.T) {
	dsu := sharedtest.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	ds, err := ExternalUpdatesOnly().CreateDataSource(basicClientContext(), dsu)
	require.NoError(t, err)
	defer ds.Close()

	assert.Equal(t, datasource.NewNullDataSource(), ds)
	assert.True(t, ds.IsInitialized())

	dsu.RequireStatusOf(t, interfaces.DataSourceStateValid)
}

package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/datasource"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/sharedtest"
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

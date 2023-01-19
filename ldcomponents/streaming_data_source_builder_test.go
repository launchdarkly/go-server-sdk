package ldcomponents

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk/v6/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingDataSourceBuilder(t *testing.T) {
	t.Run("InitialReconnectDelay", func(t *testing.T) {
		s := StreamingDataSource()
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)

		s.InitialReconnectDelay(time.Minute)
		assert.Equal(t, time.Minute, s.initialReconnectDelay)

		s.InitialReconnectDelay(0)
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)

		s.InitialReconnectDelay(-1 * time.Millisecond)
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)
	})

	t.Run("Filter", func(t *testing.T) {
		s := StreamingDataSource()
		assert.Equal(t, "", s.filterKey)

		s.Filter("microservice-1")
		assert.Equal(t, "microservice-1", s.filterKey)

		s.Filter("")
		assert.Equal(t, "", s.filterKey)
	})

	t.Run("CreateDefaultDataSource", func(t *testing.T) {
		baseURI := "base"

		s := StreamingDataSource()

		dsu := sharedtest.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataSourceUpdateSink = dsu
		ds, err := s.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		sp := ds.(*datasource.StreamProcessor)
		assert.Equal(t, baseURI, sp.GetBaseURI())
		assert.Equal(t, DefaultInitialReconnectDelay, sp.GetInitialReconnectDelay())
		assert.Equal(t, "", sp.GetFilter())
	})

	t.Run("CreateCustomizedDataSource", func(t *testing.T) {
		baseURI := "base"
		delay := time.Hour
		filter := "microservice-1"

		s := StreamingDataSource().InitialReconnectDelay(delay).Filter(filter)

		dsu := mocks.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataSourceUpdateSink = dsu
		ds, err := s.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		sp := ds.(*datasource.StreamProcessor)
		assert.Equal(t, baseURI, sp.GetBaseURI())
		assert.Equal(t, delay, sp.GetInitialReconnectDelay())
		assert.Equal(t, filter, sp.GetFilter())
	})
}

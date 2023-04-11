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

func TestPollingDataSourceBuilder(t *testing.T) {
	t.Run("PollInterval", func(t *testing.T) {
		p := PollingDataSource()
		assert.Equal(t, DefaultPollInterval, p.pollInterval)

		p.PollInterval(time.Hour)
		assert.Equal(t, time.Hour, p.pollInterval)

		p.PollInterval(time.Second)
		assert.Equal(t, DefaultPollInterval, p.pollInterval)

		p.forcePollInterval(time.Second)
		assert.Equal(t, time.Second, p.pollInterval)
	})

	t.Run("PayloadFilter", func(t *testing.T) {
		t.Run("build succeeds with no payload filter", func(t *testing.T) {
			s := PollingDataSource()
			clientContext := makeTestContextWithBaseURIs("base")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build succeeds with non-empty payload filter", func(t *testing.T) {
			s := PollingDataSource()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("microservice-1")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build fails with empty payload filter", func(t *testing.T) {
			s := PollingDataSource()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("")
			_, err := s.Build(clientContext)
			assert.Error(t, err)
		})
	})
	t.Run("CreateDefaultDataSource", func(t *testing.T) {
		baseURI := "base"

		p := PollingDataSource()

		dsu := mocks.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataSourceUpdateSink = dsu
		ds, err := p.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		pp := ds.(*datasource.PollingProcessor)
		assert.Equal(t, baseURI, pp.GetBaseURI())
		assert.Equal(t, DefaultPollInterval, pp.GetPollInterval())
	})

	t.Run("CreateCustomizedDataSource", func(t *testing.T) {
		baseURI := "base"
		interval := time.Hour
		filter := "microservice-1"

		p := PollingDataSource().PollInterval(interval).PayloadFilter(filter)

		dsu := mocks.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataSourceUpdateSink = dsu
		ds, err := p.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		pp := ds.(*datasource.PollingProcessor)
		assert.Equal(t, baseURI, pp.GetBaseURI())
		assert.Equal(t, interval, pp.GetPollInterval())
		assert.Equal(t, filter, pp.GetFilterKey())
	})
}

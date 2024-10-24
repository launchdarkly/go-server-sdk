package ldcomponents

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollingDataSourceV2Builder(t *testing.T) {
	t.Run("PollInterval", func(t *testing.T) {
		p := PollingDataSourceV2()
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
			s := PollingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build succeeds with non-empty payload filter", func(t *testing.T) {
			s := PollingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("microservice-1")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build fails with empty payload filter", func(t *testing.T) {
			s := PollingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("")
			_, err := s.Build(clientContext)
			assert.Error(t, err)
		})
	})
	t.Run("CreateDefaultDataSource", func(t *testing.T) {
		baseURI := "base"

		p := PollingDataSourceV2()

		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		statusReporter := mocks.NewMockStatusReporter()
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataDestination = dd
		clientContext.BasicClientContext.DataSourceStatusReporter = statusReporter
		ds, err := p.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		pp := ds.(*datasourcev2.PollingProcessor)
		assert.Equal(t, baseURI, pp.GetBaseURI())
		assert.Equal(t, DefaultPollInterval, pp.GetPollInterval())
	})

	t.Run("CreateCustomizedDataSource", func(t *testing.T) {
		baseURI := "base"
		interval := time.Hour
		filter := "microservice-1"

		p := PollingDataSourceV2().PollInterval(interval).PayloadFilter(filter)

		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		statusReporter := mocks.NewMockStatusReporter()
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataDestination = dd
		clientContext.BasicClientContext.DataSourceStatusReporter = statusReporter
		ds, err := p.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		pp := ds.(*datasourcev2.PollingProcessor)
		assert.Equal(t, baseURI, pp.GetBaseURI())
		assert.Equal(t, interval, pp.GetPollInterval())
		assert.Equal(t, filter, pp.GetFilterKey())
	})
}

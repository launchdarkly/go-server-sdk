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

	t.Run("CreateDataSource", func(t *testing.T) {
		baseURI := "base"
		interval := time.Hour

		p := PollingDataSource().PollInterval(interval)

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
	})
}

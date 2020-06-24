package ldcomponents

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datasource"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollingDataSourceBuilder(t *testing.T) {
	t.Run("BaseURI", func(t *testing.T) {
		p := PollingDataSource()
		assert.Equal(t, DefaultPollingBaseURI, p.baseURI)

		p.BaseURI("x")
		assert.Equal(t, "x", p.baseURI)

		p.BaseURI("")
		assert.Equal(t, DefaultPollingBaseURI, p.baseURI)
	})

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

		p := PollingDataSource().BaseURI(baseURI).PollInterval(interval)

		dsu := sharedtest.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		ds, err := p.CreateDataSource(basicClientContext(), dsu)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		pp := ds.(*datasource.PollingProcessor)
		assert.Equal(t, baseURI, pp.GetBaseURI())
		assert.Equal(t, interval, pp.GetPollInterval())
	})
}

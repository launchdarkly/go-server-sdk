package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datasource"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

func TestStreamingDataSourceBuilder(t *testing.T) {
	t.Run("BaseURI", func(t *testing.T) {
		s := StreamingDataSource()
		assert.Equal(t, DefaultStreamingBaseURI, s.baseURI)

		s.BaseURI("x")
		assert.Equal(t, "x", s.baseURI)

		s.BaseURI("")
		assert.Equal(t, DefaultStreamingBaseURI, s.baseURI)
	})

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

	t.Run("CreateDataSource", func(t *testing.T) {
		baseURI := "base"
		delay := time.Hour

		s := StreamingDataSource().BaseURI(baseURI).InitialReconnectDelay(delay)

		dsu := sharedtest.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		ds, err := s.CreateDataSource(basicClientContext(), dsu)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		sp := ds.(*datasource.StreamProcessor)
		assert.Equal(t, baseURI, sp.GetBaseURI())
		assert.Equal(t, delay, sp.GetInitialReconnectDelay())
	})
}

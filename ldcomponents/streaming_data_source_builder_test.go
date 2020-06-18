package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
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

	t.Run("PollingBaseURI", func(t *testing.T) {
		s := StreamingDataSource()
		assert.Equal(t, "", s.pollingBaseURI)

		s.PollingBaseURI("x")
		assert.Equal(t, "x", s.pollingBaseURI)

		s.PollingBaseURI("")
		assert.Equal(t, "", s.pollingBaseURI)
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
		pollingBaseURI := "poll"
		delay := time.Hour

		s := StreamingDataSource().BaseURI(baseURI).PollingBaseURI(pollingBaseURI).InitialReconnectDelay(delay)

		dsu := sharedtest.NewMockDataSourceUpdates(internal.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		ds, err := s.CreateDataSource(basicClientContext(), dsu)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		sp := ds.(*internal.StreamProcessor)
		assert.Equal(t, baseURI, sp.GetBaseURI())
		assert.Equal(t, pollingBaseURI, sp.GetPollingBaseURI())
		assert.Equal(t, delay, sp.GetInitialReconnectDelay())
	})

	t.Run("CreateDataSource can set polling base URI based on stream base URI", func(t *testing.T) {
		// If you set only BaseURI, it uses the same value for PollingBaseURI - that's the ld-relay use case.
		baseURI := "base"
		dsu := sharedtest.NewMockDataSourceUpdates(internal.NewInMemoryDataStore(sharedtest.NewTestLoggers()))

		s1 := StreamingDataSource().BaseURI(baseURI)
		ds1, err1 := s1.CreateDataSource(basicClientContext(), dsu)
		require.NoError(t, err1)
		require.NotNil(t, ds1)
		defer ds1.Close()

		sp1 := ds1.(*internal.StreamProcessor)
		assert.Equal(t, baseURI, sp1.GetBaseURI())
		assert.Equal(t, baseURI, sp1.GetPollingBaseURI())

		// If you set BaseURI, but you set it to the same value as the default, PollingBaseURI isn't changed.

		s2 := StreamingDataSource().BaseURI(DefaultStreamingBaseURI)
		ds2, err2 := s2.CreateDataSource(basicClientContext(), dsu)
		require.NoError(t, err2)
		require.NotNil(t, ds2)
		defer ds2.Close()

		sp2 := ds2.(*internal.StreamProcessor)
		assert.Equal(t, DefaultStreamingBaseURI, sp2.GetBaseURI())
		assert.Equal(t, DefaultPollingBaseURI, sp2.GetPollingBaseURI())
	})
}

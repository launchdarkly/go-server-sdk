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

func TestStreamingDataSourceV2Builder(t *testing.T) {
	t.Run("InitialReconnectDelay", func(t *testing.T) {
		s := StreamingDataSourceV2()
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)

		s.InitialReconnectDelay(time.Minute)
		assert.Equal(t, time.Minute, s.initialReconnectDelay)

		s.InitialReconnectDelay(0)
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)

		s.InitialReconnectDelay(-1 * time.Millisecond)
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)
	})

	t.Run("PayloadFilter", func(t *testing.T) {
		t.Run("build succeeds with no payload filter", func(t *testing.T) {
			s := StreamingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build succeeds with non-empty payload filter", func(t *testing.T) {
			s := StreamingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("microservice-1")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build fails with empty payload filter", func(t *testing.T) {
			s := StreamingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("")
			_, err := s.Build(clientContext)
			assert.Error(t, err)
		})
	})

	t.Run("CreateDefaultDataSource", func(t *testing.T) {
		baseURI := "base"

		s := StreamingDataSourceV2()

		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		statusReporter := mocks.NewMockStatusReporter()
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataDestination = dd
		clientContext.BasicClientContext.DataSourceStatusReporter = statusReporter
		ds, err := s.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		sp := ds.(*datasourcev2.StreamProcessor)
		assert.Equal(t, baseURI, sp.GetBaseURI())
		assert.Equal(t, DefaultInitialReconnectDelay, sp.GetInitialReconnectDelay())
		assert.Equal(t, "", sp.GetFilterKey())
	})

	t.Run("CreateCustomizedDataSource", func(t *testing.T) {
		baseURI := "base"
		delay := time.Hour
		filter := "microservice-1"

		s := StreamingDataSourceV2().InitialReconnectDelay(delay).PayloadFilter(filter)

		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		statusReporter := mocks.NewMockStatusReporter()
		clientContext := makeTestContextWithBaseURIs(baseURI)
		clientContext.BasicClientContext.DataDestination = dd
		clientContext.BasicClientContext.DataSourceStatusReporter = statusReporter
		ds, err := s.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		sp := ds.(*datasourcev2.StreamProcessor)
		assert.Equal(t, baseURI, sp.GetBaseURI())
		assert.Equal(t, delay, sp.GetInitialReconnectDelay())
		assert.Equal(t, filter, sp.GetFilterKey())
	})
}

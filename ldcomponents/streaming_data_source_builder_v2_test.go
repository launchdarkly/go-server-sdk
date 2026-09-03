package ldcomponents

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

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

		t.Run("build succeeds with empty payload filter", func(t *testing.T) {
			s := StreamingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build logs a deprecation warning when a payload filter is set", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			clientContext := makeTestContextWithBaseURIs("base")
			clientContext.Logging = subsystems.LoggingConfiguration{Loggers: mockLog.Loggers}

			_, err := StreamingDataSourceV2().PayloadFilter("microservice-1").Build(clientContext)
			require.NoError(t, err)
			mockLog.AssertMessageMatch(t, true, ldlog.Warn, "Payload filtering is not supported")

			mockLog2 := ldlogtest.NewMockLog()
			clientContext.Logging = subsystems.LoggingConfiguration{Loggers: mockLog2.Loggers}

			_, err = StreamingDataSourceV2().Build(clientContext)
			require.NoError(t, err)
			mockLog2.AssertMessageMatch(t, false, ldlog.Warn, "Payload filtering is not supported")
		})
	})

	t.Run("DoesNotUseBaseURIFromContext", func(t *testing.T) {
		fdv1BaseURI := "base"

		// A default FDv2 streaming data source configures itself with the default
		// streaming URI internally - it doesn't look in the Context's ServiceEndpoints for it. This is because
		// custom endpoints are injected within the Data System config.
		s := StreamingDataSourceV2()

		statusReporter := mocks.NewMockStatusReporter()

		clientContext := makeTestContextWithBaseURIs(fdv1BaseURI)
		clientContext.BasicClientContext.DataSourceStatusReporter = statusReporter

		ds, err := s.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		sp := ds.(*datasourcev2.StreamProcessor)
		assert.Equal(t, DefaultStreamingBaseURI, sp.GetBaseURI())
		assert.Equal(t, DefaultInitialReconnectDelay, sp.GetInitialReconnectDelay())
		assert.Equal(t, "", sp.GetFilterKey())
	})

	t.Run("CreateCustomizedDataSource", func(t *testing.T) {
		baseURI := "base-uri"
		delay := time.Hour
		filter := "microservice-1"

		s := StreamingDataSourceV2().InitialReconnectDelay(delay).PayloadFilter(filter).BaseURI(baseURI)

		statusReporter := mocks.NewMockStatusReporter()
		clientContext := makeTestContextWithBaseURIs("not-used")
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

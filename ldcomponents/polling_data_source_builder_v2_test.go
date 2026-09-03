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

		t.Run("build succeeds with empty payload filter", func(t *testing.T) {
			s := PollingDataSourceV2()
			clientContext := makeTestContextWithBaseURIs("base")
			s.PayloadFilter("")
			_, err := s.Build(clientContext)
			assert.NoError(t, err)
		})

		t.Run("build logs a deprecation warning when a payload filter is set", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			clientContext := makeTestContextWithBaseURIs("base")
			clientContext.Logging = subsystems.LoggingConfiguration{Loggers: mockLog.Loggers}

			_, err := PollingDataSourceV2().PayloadFilter("microservice-1").Build(clientContext)
			require.NoError(t, err)
			mockLog.AssertMessageMatch(t, true, ldlog.Warn, "Payload filtering is not supported")

			mockLog2 := ldlogtest.NewMockLog()
			clientContext.Logging = subsystems.LoggingConfiguration{Loggers: mockLog2.Loggers}

			_, err = PollingDataSourceV2().Build(clientContext)
			require.NoError(t, err)
			mockLog2.AssertMessageMatch(t, false, ldlog.Warn, "Payload filtering is not supported")
		})
	})
	t.Run("DoesNotUseBaseURIFromContext", func(t *testing.T) {
		fdv1BaseURI := "base"

		// A default FDv2 polling data source configures itself with the default
		// polling URI internally - it doesn't look in the Context's ServiceEndpoints for it. This is because
		// custom endpoints are injected within the Data System config.
		p := PollingDataSourceV2()

		statusReporter := mocks.NewMockStatusReporter()

		clientContext := makeTestContextWithBaseURIs(fdv1BaseURI)
		clientContext.BasicClientContext.DataSourceStatusReporter = statusReporter

		ds, err := p.Build(clientContext)
		require.NoError(t, err)
		require.NotNil(t, ds)
		defer ds.Close()

		pp := ds.(*datasourcev2.PollingProcessor)
		assert.Equal(t, DefaultPollingBaseURI, pp.GetBaseURI())
		assert.Equal(t, DefaultPollInterval, pp.GetPollInterval())
	})

	t.Run("CreateCustomizedDataSource", func(t *testing.T) {
		baseURI := "base"
		interval := time.Hour
		filter := "microservice-1"

		p := PollingDataSourceV2().PollInterval(interval).PayloadFilter(filter).BaseURI(baseURI)

		statusReporter := mocks.NewMockStatusReporter()

		clientContext := makeTestContextWithBaseURIs(baseURI)
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

package ldcomponents

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v6/internal/endpoints"
)

// DefaultPollingBaseURI is the default value for PollingDataSourceBuilder.BaseURI.
const DefaultPollingBaseURI = "https://app.launchdarkly.com"

// DefaultPollInterval is the default value for PollingDataSourceBuilder.PollInterval. This is also the minimum value.
const DefaultPollInterval = 30 * time.Second

// PollingDataSourceBuilder provides methods for configuring the polling data source.
//
// See PollingDataSource for usage.
type PollingDataSourceBuilder struct {
	baseURI      string
	pollInterval time.Duration
}

// PollingDataSource returns a configurable factory for using polling mode to get feature flag data.
//
// Polling is not the default behavior; by default, the SDK uses a streaming connection to receive feature flag
// data from LaunchDarkly. In polling mode, the SDK instead makes a new HTTP request to LaunchDarkly at regular
// intervals. HTTP caching allows it to avoid redundantly downloading data if there have been no changes, but
// polling is still less efficient than streaming and should only be used on the advice of LaunchDarkly support.
//
// To use polling mode, create a builder with PollingDataSource(), set its properties with the methods of
// PollingDataSourceBuilder, and then store it in the DataSource field of your SDK configuration:
//
//     config := ld.Config{
//         DataSource: ldcomponents.PollingDataSource().PollInterval(45 * time.Second),
//     }
func PollingDataSource() *PollingDataSourceBuilder {
	return &PollingDataSourceBuilder{
		pollInterval: DefaultPollInterval,
	}
}

// PollInterval sets the interval at which the SDK will poll for feature flag updates.
//
// The default and minimum value is DefaultPollInterval. Values less than this will be set to the default.
func (b *PollingDataSourceBuilder) PollInterval(pollInterval time.Duration) *PollingDataSourceBuilder {
	if pollInterval < DefaultPollInterval {
		b.pollInterval = DefaultPollInterval
	} else {
		b.pollInterval = pollInterval
	}
	return b
}

// Used in tests to skip parameter validation.
//nolint:unused // it is used in tests
func (b *PollingDataSourceBuilder) forcePollInterval(
	pollInterval time.Duration,
) *PollingDataSourceBuilder {
	b.pollInterval = pollInterval
	return b
}

// CreateDataSource is called by the SDK to create the data source instance.
func (b *PollingDataSourceBuilder) CreateDataSource(
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	context.GetLogging().Loggers.Warn(
		"You should only disable the streaming API if instructed to do so by LaunchDarkly support")
	configuredBaseURI := endpoints.SelectBaseURI(
		context.GetBasic().ServiceEndpoints,
		endpoints.PollingService,
		b.baseURI,
		context.GetLogging().Loggers,
	)
	pp := datasource.NewPollingProcessor(context, dataSourceUpdates, configuredBaseURI, b.pollInterval)
	return pp, nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *PollingDataSourceBuilder) DescribeConfiguration(context interfaces.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		SetBool("streamingDisabled", true).
		SetBool("customBaseURI",
			endpoints.IsCustom(context.GetBasic().ServiceEndpoints, endpoints.PollingService, b.baseURI)).
		Set("pollingIntervalMillis", durationToMillisValue(b.pollInterval)).
		SetBool("usingRelayDaemon", false).
		Build()
}

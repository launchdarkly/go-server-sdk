package ldcomponents

import (
	"errors"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// DefaultPollingBaseURI is the default value for [PollingDataSourceBuilder.BaseURI].
const DefaultPollingBaseURI = "https://app.launchdarkly.com"

// DefaultPollInterval is the default value for [PollingDataSourceBuilder.PollInterval]. This is also the minimum value.
const DefaultPollInterval = 30 * time.Second

// PollingDataSourceBuilder provides methods for configuring the polling data source.
//
// See [PollingDataSource] for usage.
type PollingDataSourceBuilder struct {
	pollInterval time.Duration
	filterKey    ldvalue.OptionalString
}

// PollingDataSource returns a configurable factory for using polling mode to get feature flag data.
//
// Polling is not the default behavior; by default, the SDK uses a streaming connection to receive feature flag
// data from LaunchDarkly. In polling mode, the SDK instead makes a new HTTP request to LaunchDarkly at regular
// intervals. HTTP caching allows it to avoid redundantly downloading data if there have been no changes, but
// polling is still less efficient than streaming and should only be used on the advice of LaunchDarkly support.
//
// To use polling mode, create a builder with PollingDataSource(), set its properties with the methods of
// [PollingDataSourceBuilder], and then store it in the DataSource field of
// [github.com/launchdarkly/go-server-sdk/v7.Config]:
//
//	config := ld.Config{
//	    DataSource: ldcomponents.PollingDataSource().PollInterval(45 * time.Second),
//	}
func PollingDataSource() *PollingDataSourceBuilder {
	return &PollingDataSourceBuilder{
		pollInterval: DefaultPollInterval,
	}
}

// PollInterval sets the interval at which the SDK will poll for feature flag updates.
//
// The default and minimum value is [DefaultPollInterval]. Values less than this will be set to the default.
func (b *PollingDataSourceBuilder) PollInterval(pollInterval time.Duration) *PollingDataSourceBuilder {
	if pollInterval < DefaultPollInterval {
		b.pollInterval = DefaultPollInterval
	} else {
		b.pollInterval = pollInterval
	}
	return b
}

// Used in tests to skip parameter validation.
//
//nolint:unused // it is used in tests
func (b *PollingDataSourceBuilder) forcePollInterval(
	pollInterval time.Duration,
) *PollingDataSourceBuilder {
	b.pollInterval = pollInterval
	return b
}

// PayloadFilter sets the filter key for the polling connection.
//
// By default, the SDK is able to evaluate all flags in an environment. If this is undesirable -
// for example, the environment contains thousands of flags, but this application only needs to evaluate
// a smaller, known subset - then a filter may be setup in LaunchDarkly, and the filter's key specified here.
//
// Evaluations for flags that aren't part of the filtered environment will return default values.
func (b *PollingDataSourceBuilder) PayloadFilter(filterKey string) *PollingDataSourceBuilder {
	b.filterKey = ldvalue.NewOptionalString(filterKey)
	return b
}

// Build is called internally by the SDK.
func (b *PollingDataSourceBuilder) Build(context subsystems.ClientContext) (subsystems.DataSource, error) {
	context.GetLogging().Loggers.Warn(
		"You should only disable the streaming API if instructed to do so by LaunchDarkly support")
	filterKey, wasSet := b.filterKey.Get()
	if wasSet && filterKey == "" {
		return nil, errors.New("payload filter key cannot be an empty string")
	}
	configuredBaseURI := endpoints.SelectBaseURI(
		context.GetServiceEndpoints(),
		endpoints.PollingService,
		context.GetLogging().Loggers,
	)
	cfg := datasource.PollingConfig{
		BaseURI:      configuredBaseURI,
		PollInterval: b.pollInterval,
		FilterKey:    filterKey,
	}
	pp := datasource.NewPollingProcessor(context, context.GetDataSourceUpdateSink(), cfg)
	return pp, nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *PollingDataSourceBuilder) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		SetBool("streamingDisabled", true).
		SetBool("customBaseURI",
			endpoints.IsCustom(context.GetServiceEndpoints(), endpoints.PollingService)).
		Set("pollingIntervalMillis", durationToMillisValue(b.pollInterval)).
		SetBool("usingRelayDaemon", false).
		Build()
}

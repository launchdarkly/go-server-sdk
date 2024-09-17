package ldcomponents

import (
	"errors"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// PollingDataSourceBuilderV2 provides methods for configuring the polling data source.
//
// This builder is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type PollingDataSourceBuilderV2 struct {
	pollInterval time.Duration
	filterKey    ldvalue.OptionalString
}

// PollingDataSourceV2 returns a configurable factory for using polling mode to get feature flag data.
//
// This builder is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
//
// Polling is not the default behavior; by default, the SDK uses a streaming connection to receive feature flag
// data from LaunchDarkly. In polling mode, the SDK instead makes a new HTTP request to LaunchDarkly at regular
// intervals. HTTP caching allows it to avoid redundantly downloading data if there have been no changes, but
// polling is still less efficient than streaming and should only be used on the advice of LaunchDarkly support.
func PollingDataSourceV2() *PollingDataSourceBuilderV2 {
	return &PollingDataSourceBuilderV2{
		pollInterval: DefaultPollInterval,
	}
}

// PollInterval sets the interval at which the SDK will poll for feature flag updates.
//
// The default and minimum value is [DefaultPollInterval]. Values less than this will be set to the default.
func (b *PollingDataSourceBuilderV2) PollInterval(pollInterval time.Duration) *PollingDataSourceBuilderV2 {
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
func (b *PollingDataSourceBuilderV2) forcePollInterval(
	pollInterval time.Duration,
) *PollingDataSourceBuilderV2 {
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
func (b *PollingDataSourceBuilderV2) PayloadFilter(filterKey string) *PollingDataSourceBuilderV2 {
	b.filterKey = ldvalue.NewOptionalString(filterKey)
	return b
}

// Build is called internally by the SDK.
func (b *PollingDataSourceBuilderV2) Build(context subsystems.ClientContext) (subsystems.DataSource, error) {
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
	return datasourcev2.NewPollingProcessor(context, context.GetDataSourceUpdateSink(), cfg), nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *PollingDataSourceBuilderV2) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		SetBool("streamingDisabled", true).
		SetBool("customBaseURI",
			endpoints.IsCustom(context.GetServiceEndpoints(), endpoints.PollingService)).
		Set("pollingIntervalMillis", durationToMillisValue(b.pollInterval)).
		SetBool("usingRelayDaemon", false).
		Build()
}

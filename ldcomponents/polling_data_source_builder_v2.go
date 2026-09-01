package ldcomponents

import (
	"errors"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// PollingDataSourceBuilderV2 provides methods for configuring the polling data source.
type PollingDataSourceBuilderV2 struct {
	pollInterval time.Duration
	filterKey    ldvalue.OptionalString
	baseURI      string
}

// PollingDataSourceV2 returns a configurable factory for using polling mode to get feature flag data.
// Polling is not the default behavior; by default, the SDK uses a streaming connection to receive feature flag
// data from LaunchDarkly. In polling mode, the SDK instead makes a new HTTP request to LaunchDarkly at regular
// intervals. HTTP caching allows it to avoid redundantly downloading data if there have been no changes, but
// polling is still less efficient than streaming and should only be used on the advice of LaunchDarkly support.
func PollingDataSourceV2() *PollingDataSourceBuilderV2 {
	return &PollingDataSourceBuilderV2{
		pollInterval: DefaultPollInterval,
		baseURI:      DefaultPollingBaseURI,
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

// BaseURI sets the base URI for the polling connection.
func (b *PollingDataSourceBuilderV2) BaseURI(baseURI string) *PollingDataSourceBuilderV2 {
	b.baseURI = baseURI
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
//
// Deprecated: Payload filtering is not supported with the FDv2 data system and this method will be
// removed in a future release. There is no replacement: payload filtering is only available with
// the FDv1 data source.
func (b *PollingDataSourceBuilderV2) PayloadFilter(filterKey string) *PollingDataSourceBuilderV2 {
	b.filterKey = ldvalue.NewOptionalString(filterKey)
	return b
}

// Build is called internally by the SDK.
func (b *PollingDataSourceBuilderV2) Build(context subsystems.ClientContext) (subsystems.DataSynchronizer, error) {
	filterKey, wasSet := b.filterKey.Get()
	if wasSet && filterKey == "" {
		return nil, errors.New("payload filter key cannot be an empty string")
	}
	cfg := datasource.PollingConfig{
		BaseURI:      b.baseURI,
		PollInterval: b.pollInterval,
		FilterKey:    filterKey,
	}
	return datasourcev2.NewPollingProcessor(context, cfg), nil
}

// AsInitializer converts the builder into a component configurer for a data initializer. The purpose
// is to allow the PollingDataSourceBuilderV2, which is normally a synchronizer, to be used as an initializer.
func (b *PollingDataSourceBuilderV2) AsInitializer() subsystems.ComponentConfigurer[subsystems.DataInitializer] {
	return subsystems.AsInitializer(b)
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *PollingDataSourceBuilderV2) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		SetBool("streamingDisabled", true).
		SetBool("customBaseURI",
			b.baseURI != DefaultPollingBaseURI).
		Set("pollingIntervalMillis", durationToMillisValue(b.pollInterval)).
		SetBool("usingRelayDaemon", false).
		Build()
}

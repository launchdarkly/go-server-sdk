package ldcomponents

import (
	"errors"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// FDv1PollingDataSourceBuilderV2 provides methods for configuring the polling
// data source that relies on the fdv1 endpoints.
//
// This builder is not stable, and not subject to any backwards compatibility
// guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type FDv1PollingDataSourceBuilderV2 struct {
	pollInterval time.Duration
	filterKey    ldvalue.OptionalString
	baseURI      string
}

// FDv1PollingDataSourceV2 returns a configurable factory for using polling
// mode to get feature flag data from the old fdv1 endpoint.
//
// This builder is not stable, and not subject to any backwards compatibility
// guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
func FDv1PollingDataSourceV2() *FDv1PollingDataSourceBuilderV2 {
	return &FDv1PollingDataSourceBuilderV2{
		pollInterval: DefaultPollInterval,
		baseURI:      DefaultPollingBaseURI,
	}
}

// PollInterval sets the interval at which the SDK will poll for feature flag updates.
//
// The default and minimum value is [DefaultPollInterval]. Values less than this will be set to the default.
func (b *FDv1PollingDataSourceBuilderV2) PollInterval(pollInterval time.Duration) *FDv1PollingDataSourceBuilderV2 {
	b.pollInterval = max(pollInterval, DefaultPollInterval)
	return b
}

// BaseURI sets the base URI for the polling connection.
func (b *FDv1PollingDataSourceBuilderV2) BaseURI(baseURI string) *FDv1PollingDataSourceBuilderV2 {
	b.baseURI = baseURI
	return b
}

// PayloadFilter sets the filter key for the polling connection.
//
// By default, the SDK is able to evaluate all flags in an environment. If this is undesirable -
// for example, the environment contains thousands of flags, but this application only needs to evaluate
// a smaller, known subset - then a filter may be setup in LaunchDarkly, and the filter's key specified here.
//
// Evaluations for flags that aren't part of the filtered environment will return default values.
func (b *FDv1PollingDataSourceBuilderV2) PayloadFilter(filterKey string) *FDv1PollingDataSourceBuilderV2 {
	b.filterKey = ldvalue.NewOptionalString(filterKey)
	return b
}

// Build is called internally by the SDK.
func (b *FDv1PollingDataSourceBuilderV2) Build(context subsystems.ClientContext) (subsystems.DataSynchronizer, error) {
	context.GetLogging().Loggers.Warn(
		"You should only disable the streaming API if instructed to do so by LaunchDarkly support")
	filterKey, wasSet := b.filterKey.Get()
	if wasSet && filterKey == "" {
		return nil, errors.New("payload filter key cannot be an empty string")
	}
	cfg := datasource.PollingConfig{
		BaseURI:      b.baseURI,
		PollInterval: b.pollInterval,
		FilterKey:    filterKey,
	}
	return datasourcev2.NewFDv1PollingProcessor(context, context.GetDataDestination(), cfg), nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *FDv1PollingDataSourceBuilderV2) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		SetBool("streamingDisabled", true).
		SetBool("customBaseURI",
			b.baseURI != DefaultPollingBaseURI).
		Set("pollingIntervalMillis", durationToMillisValue(b.pollInterval)).
		SetBool("usingRelayDaemon", false).
		Build()
}

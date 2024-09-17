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

// StreamingDataSourceBuilderV2 provides methods for configuring the streaming data source in v2 mode.
//
// This builder is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type StreamingDataSourceBuilderV2 struct {
	initialReconnectDelay time.Duration
	filterKey             ldvalue.OptionalString
}

// StreamingDataSourceV2 returns a configurable factory for using streaming mode to get feature flag data.
//
// This builder is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
//
// By default, the SDK uses a streaming connection to receive feature flag data from LaunchDarkly. To use the
// default behavior, you do not need to call this method.
func StreamingDataSourceV2() *StreamingDataSourceBuilderV2 {
	return &StreamingDataSourceBuilderV2{
		initialReconnectDelay: DefaultInitialReconnectDelay,
	}
}

// InitialReconnectDelay sets the initial reconnect delay for the streaming connection.
//
// The streaming service uses a backoff algorithm (with jitter) every time the connection needs to be
// reestablished. The delay for the first reconnection will start near this value, and then increase
// exponentially for any subsequent connection failures.
//
// The default value is [DefaultInitialReconnectDelay].
func (b *StreamingDataSourceBuilderV2) InitialReconnectDelay(
	initialReconnectDelay time.Duration,
) *StreamingDataSourceBuilderV2 {
	if initialReconnectDelay <= 0 {
		b.initialReconnectDelay = DefaultInitialReconnectDelay
	} else {
		b.initialReconnectDelay = initialReconnectDelay
	}
	return b
}

// PayloadFilter sets the payload filter key for this streaming connection. The filter key
// cannot be an empty string.
//
// By default, the SDK is able to evaluate all flags in an environment. If this is undesirable -
// for example, the environment contains thousands of flags, but this application only needs to evaluate
// a smaller, known subset - then a payload filter may be setup in LaunchDarkly, and the filter's key specified here.
//
// Evaluations for flags that aren't part of the filtered environment will return default values.
func (b *StreamingDataSourceBuilderV2) PayloadFilter(filterKey string) *StreamingDataSourceBuilderV2 {
	b.filterKey = ldvalue.NewOptionalString(filterKey)
	return b
}

// Build is called internally by the SDK.
func (b *StreamingDataSourceBuilderV2) Build(context subsystems.ClientContext) (subsystems.DataSource, error) {
	filterKey, wasSet := b.filterKey.Get()
	if wasSet && filterKey == "" {
		return nil, errors.New("payload filter key cannot be an empty string")
	}
	configuredBaseURI := endpoints.SelectBaseURI(
		context.GetServiceEndpoints(),
		endpoints.StreamingService,
		context.GetLogging().Loggers,
	)
	cfg := datasource.StreamConfig{
		URI:                   configuredBaseURI,
		InitialReconnectDelay: b.initialReconnectDelay,
		FilterKey:             filterKey,
	}
	return datasourcev2.NewStreamProcessor(
		context,
		context.GetDataSourceUpdateSink(),
		cfg,
	), nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *StreamingDataSourceBuilderV2) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		SetBool("streamingDisabled", false).
		SetBool("customStreamURI",
			endpoints.IsCustom(context.GetServiceEndpoints(), endpoints.StreamingService)).
		Set("reconnectTimeMillis", durationToMillisValue(b.initialReconnectDelay)).
		SetBool("usingRelayDaemon", false).
		Build()
}

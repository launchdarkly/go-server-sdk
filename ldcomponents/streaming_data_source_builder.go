package ldcomponents

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v6/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// DefaultStreamingBaseURI is the default value for [StreamingDataSourceBuilder.BaseURI].
const DefaultStreamingBaseURI = endpoints.DefaultStreamingBaseURI

// DefaultInitialReconnectDelay is the default value for [StreamingDataSourceBuilder.InitialReconnectDelay].
const DefaultInitialReconnectDelay = time.Second

// DefaultFilterKey is the default value for [StreamingDataSourceBuilder.Filter]. The empty string
// signifies that no filter should be used.
const DefaultFilterKey = ""

// StreamingDataSourceBuilder provides methods for configuring the streaming data source.
//
// See StreamingDataSource for usage.
type StreamingDataSourceBuilder struct {
	baseURI               string
	initialReconnectDelay time.Duration
	filterKey             string
}

// StreamingDataSource returns a configurable factory for using streaming mode to get feature flag data.
//
// By default, the SDK uses a streaming connection to receive feature flag data from LaunchDarkly. To use the
// default behavior, you do not need to call this method. However, if you want to customize the behavior of
// the connection, call this method to obtain a builder, set its properties with the [StreamingDataSourceBuilder]
// methods, and then store it in the DataSource field of [github.com/launchdarkly/go-server-sdk/v6.Config]:
//
//	config := ld.Config{
//	    DataSource: ldcomponents.StreamingDataSource().InitialReconnectDelay(500 * time.Millisecond),
//	}
func StreamingDataSource() *StreamingDataSourceBuilder {
	return &StreamingDataSourceBuilder{
		initialReconnectDelay: DefaultInitialReconnectDelay,
		filterKey:             DefaultFilterKey,
	}
}

// InitialReconnectDelay sets the initial reconnect delay for the streaming connection.
//
// The streaming service uses a backoff algorithm (with jitter) every time the connection needs to be
// reestablished. The delay for the first reconnection will start near this value, and then increase
// exponentially for any subsequent connection failures.
//
// The default value is [DefaultInitialReconnectDelay].
func (b *StreamingDataSourceBuilder) InitialReconnectDelay(
	initialReconnectDelay time.Duration,
) *StreamingDataSourceBuilder {
	if initialReconnectDelay <= 0 {
		b.initialReconnectDelay = DefaultInitialReconnectDelay
	} else {
		b.initialReconnectDelay = initialReconnectDelay
	}
	return b
}

// Filter sets the filter key for the streaming connection.
//
// By default, the SDK is able to evaluate all flags in an environment. If this is undesirable -
// for example, the environment contains thousands of flags, but this application only needs to evaluate
// a smaller, known subset - then a filter may be setup in LaunchDarkly, and the filter's key specified here.
//
// Evaluations for flags that aren't part of the filtered environment will return default values.
func (b *StreamingDataSourceBuilder) Filter(filterKey string) *StreamingDataSourceBuilder {
	b.filterKey = filterKey
	return b
}

// Build is called internally by the SDK.
func (b *StreamingDataSourceBuilder) Build(context subsystems.ClientContext) (subsystems.DataSource, error) {
	configuredBaseURI := endpoints.SelectBaseURI(
		context.GetServiceEndpoints(),
		endpoints.StreamingService,
		b.baseURI,
		context.GetLogging().Loggers,
	)
	cfg := datasource.StreamConfig{
		URI:                   configuredBaseURI,
		InitialReconnectDelay: b.initialReconnectDelay,
		FilterKey:             b.filterKey,
	}
	return datasource.NewStreamProcessor(
		context,
		context.GetDataSourceUpdateSink(),
		cfg,
	), nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *StreamingDataSourceBuilder) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		SetBool("streamingDisabled", false).
		SetBool("customStreamURI",
			endpoints.IsCustom(context.GetServiceEndpoints(), endpoints.StreamingService, b.baseURI)).
		Set("reconnectTimeMillis", durationToMillisValue(b.initialReconnectDelay)).
		SetBool("usingRelayDaemon", false).
		Build()
}

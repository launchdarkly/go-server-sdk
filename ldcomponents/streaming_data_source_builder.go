package ldcomponents

import (
	"strings"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// DefaultStreamingBaseURI is the default value for StreamingDataSourceBuilder.BaseURI.
const DefaultStreamingBaseURI = "https://stream.launchdarkly.com"

// DefaultInitialReconnectDelay is the default value for StreamingDataSourceBuilder.InitialReconnectDelay.
const DefaultInitialReconnectDelay = time.Second

// StreamingDataSourceBuilder provides methods for configuring the streaming data source.
//
// See StreamingDataSource for usage.
type StreamingDataSourceBuilder struct {
	baseURI               string
	pollingBaseURI        string
	initialReconnectDelay time.Duration
}

// StreamingDataSource returns a configurable factory for using streaming mode to get feature flag data.
//
// By default, the SDK uses a streaming connection to receive feature flag data from LaunchDarkly. To use the
// default behavior, you do not need to call this method. However, if you want to customize the behavior of
// the connection, call this method to obtain a builder, set its properties with the StreamingDataSourceBuilder
// methods, and then store it in the DataSource field of your SDK configuration:
//
//     config := ld.Config{
//         DataSource: ldcomponents.StreamingDataSource().InitialReconnectDelay(500 * time.Millisecond),
//     }
func StreamingDataSource() *StreamingDataSourceBuilder {
	return &StreamingDataSourceBuilder{
		baseURI:               DefaultStreamingBaseURI,
		initialReconnectDelay: DefaultInitialReconnectDelay,
	}
}

// BaseURI sets a custom base URI for the streaming service.
//
// You will only need to change this value in the following cases:
//
// 1. You are using the Relay Proxy (https://docs.launchdarkly.com/docs/the-relay-proxy). Set BaseURI to the base URI of
// the Relay Proxy instance.
//
// 2. You are connecting to a test server or anything else other than the standard LaunchDarkly service.
func (b *StreamingDataSourceBuilder) BaseURI(baseURI string) *StreamingDataSourceBuilder {
	if baseURI == "" {
		b.baseURI = DefaultStreamingBaseURI
	} else {
		b.baseURI = strings.TrimRight(baseURI, "/")
	}
	return b
}

// PollingBaseURI sets a custom base URI for special polling requests.
//
// Even in streaming mode, the SDK sometimes temporarily must do a polling request. You do not need to
// modify this property unless you are connecting to a test server or a nonstandard endpoint for the
// LaunchDarkly service. If you are using the Relay Proxy (https://docs.launchdarkly.com/docs/the-relay-proxy),
// you only need to set BaseURI.
func (b *StreamingDataSourceBuilder) PollingBaseURI(pollingBaseURI string) *StreamingDataSourceBuilder {
	b.pollingBaseURI = strings.TrimRight(pollingBaseURI, "/")
	return b
}

// InitialReconnectDelay sets the initial reconnect delay for the streaming connection.
//
// The streaming service uses a backoff algorithm (with jitter) every time the connection needs to be
// reestablished. The delay for the first reconnection will start near this value, and then increase
// exponentially for any subsequent connection failures.
//
// The default value is DefaultInitialReconnectDelay.
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

// CreateDataSource is called by the SDK to create the data source instance.
func (b *StreamingDataSourceBuilder) CreateDataSource(
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	pollingBaseURI := b.pollingBaseURI
	if pollingBaseURI == "" {
		if b.baseURI == DefaultStreamingBaseURI {
			pollingBaseURI = DefaultPollingBaseURI
		} else {
			// There's a custom stream URI, so it is probably a Relay instance and the polling base URI should
			// default to the same value.
			pollingBaseURI = b.baseURI
		}
	}
	requestor := newRequestor(context, context.GetHTTP().CreateHTTPClient(), pollingBaseURI)
	return newStreamProcessor(context, dataSourceUpdates, b.baseURI, b.initialReconnectDelay, requestor), nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *StreamingDataSourceBuilder) DescribeConfiguration() ldvalue.Value {
	isCustomStreamURI := b.baseURI != "" && b.baseURI != DefaultStreamingBaseURI
	isCustomBaseURI := (b.pollingBaseURI != "" && b.pollingBaseURI != DefaultPollingBaseURI) ||
		(b.pollingBaseURI == "" && b.baseURI != DefaultStreamingBaseURI)
	return ldvalue.ObjectBuild().
		Set("streamingDisabled", ldvalue.Bool(false)).
		Set("customBaseURI", ldvalue.Bool(isCustomBaseURI)).
		Set("customStreamURI", ldvalue.Bool(isCustomStreamURI)).
		Set("reconnectTimeMillis", ldvalue.Int(int(b.initialReconnectDelay/time.Millisecond))).
		Set("usingRelayDaemon", ldvalue.Bool(false)).
		Build()
}

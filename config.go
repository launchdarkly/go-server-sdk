package ldclient

import (
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
)

// Config exposes advanced configuration options for the LaunchDarkly client.
//
// All of these settings are optional, so an empty Config struct is always valid. See the description of each
// field for the default behavior if it is not set.
//
// Some of the Config fields are actually factories for subcomponents of the SDK. The types of these fields
// are interfaces whose names end in "Factory"; the actual implementation types, which have methods for
// configuring that subcomponent, are normally provided by corresponding functions in the ldcomponents
// package (https://pkg.go.dev/github.com/launchdarkly/go-server-sdk/v6/ldcomponents). For instance, to set
// the Events field to a configuration in which the SDK will flush analytics events every 10 seconds:
//
//     var config ld.Config
//     config.Events = ldcomponents.Events().FlushInterval(time.Second * 10)
//
// The interfaces are defined separately from the built-in component implementations because you could also
// define your own implementation, for custom SDK integrations.
type Config struct {
	// Provides configuration of the SDK's Big Segments feature.
	//
	// "Big Segments" are a specific type of user segments. For more information, read the LaunchDarkly
	// documentation about user segments: https://docs.launchdarkly.com/home/users
	//
	// If you are using this feature, you will normally specify a database implementation that matches how
	// the LaunchDarkly Relay Proxy is configured, since the Relay Proxy manages the Big Segment data.
	//
	// If nil, there is no implementation and Big Segments cannot be evaluated. In this case, any flag
	// evaluation that references a Big Segment will behave as if no users are included in any Big
	// Segments, and the EvaluationReason associated with any such flag evaluation will return
	// ldreason.BigSegmentsStoreNotConfigured from its GetBigSegmentsStatus() method.
	//
	//     // example: use Redis, with default properties
	//     import ldredis "github.com/launchdarkly/go-server-sdk-redis-redigo"
	//
	//     config.BigSegmentStore = ldcomponents.BigSegments(ldredis.DataStore())
	BigSegments interfaces.BigSegmentsConfigurationFactory

	// Sets the implementation of DataSource for receiving feature flag updates.
	//
	// If nil, the default is ldcomponents.StreamingDataSource(); see that method for an explanation of how to
	// further configure streaming behavior. Other options include ldcomponents.PollingDataSource(),
	// ldcomponents.ExternalUpdatesOnly(), ldfiledata.DataSource(), or a custom implementation for testing.
	//
	// If Offline is set to true, then DataSource is ignored.
	//
	//     // example: using streaming mode and setting streaming options
	//     config.DataSource = ldcomponents.StreamingDataSource().InitialReconnectDelay(time.Second)
	//
	//     // example: using polling mode and setting polling options
	//     config.DataSource = ldcomponents.PollingDataSource().PollInterval(time.Minute)
	//
	//     // example: specifying that data will be updated by an external process (such as the Relay Proxy)
	//     config.DataSource = ldcomponents.ExternalUpdatesOnly()
	DataSource interfaces.DataSourceFactory

	// Sets the implementation of DataStore for holding feature flags and related data received from
	// LaunchDarkly.
	//
	// If nil, the default is ldcomponents.InMemoryDataStore().
	//
	// The other option is to use a persistent data store-- that is, a database integration. These all use
	// ldcomponents.PersistentDataStore(), plus an adapter for the specific database. LaunchDarkly provides
	// adapters for several databases, as described in the Reference Guide:
	// https://docs.launchdarkly.com/sdk/concepts/data-stores
	//
	// You could also define your own database integration by implementing the PersistentDataStore interface.
	//
	//     // example: use Redis, with default properties
	//     import ldredis "github.com/launchdarkly/go-server-sdk-redis-redigo"
	//
	//     config.DataStore = ldcomponents.PersistentDataStore(ldredis.DataStore())
	DataStore interfaces.DataStoreFactory

	// Set to true to opt out of sending diagnostic events.
	//
	// Unless DiagnosticOptOut is set to true, the client will send some diagnostics data to the LaunchDarkly
	// servers in order to assist in the development of future SDK improvements. These diagnostics consist of an
	// initial payload containing some details of the SDK in use, the SDK's configuration, and the platform the
	// SDK is being run on, as well as payloads sent periodically with information on irregular occurrences such
	// as dropped events.
	DiagnosticOptOut bool

	// Sets the SDK's behavior regarding analytics events.
	//
	// If nil, the default is ldcomponents.SendEvents(); see that method for an explanation of how to further
	// configure event delivery. You may also turn off event delivery using ldcomponents.NoEvents().
	//
	// If Offline is set to true, then event delivery is always off and Events is ignored.
	//
	//     // example: enable events, flush the events every 10 seconds, buffering up to 5000 events
	//     config.Events = ldcomponents.SendEvents().FlushInterval(10 * time.Second).Capacity(5000)
	Events interfaces.EventProcessorFactory

	// Provides configuration of the SDK's network connection behavior.
	//
	// If nil, the default is ldcomponents.HTTPConfiguration(); see that method for an explanation of how to
	// further configure these options.
	//
	// If Offline is set to true, then HTTP is ignored.
	//
	//     // example: set connection timeout to 8 seconds and use a proxy server
	//     config.HTTP = ldcomponents.HTTPConfiguration().ConnectTimeout(8 * time.Second).ProxyURL(myProxyURL)
	HTTP interfaces.HTTPConfigurationFactory

	// Provides configuration of the SDK's logging behavior.
	//
	// If nil, the default is ldcomponents.Logging(); see that method for an explanation of how to
	// further configure logging behavior. The other option is ldcomponents.NoLogging().
	//
	// This example sets the minimum logging level to Warn, so Debug and Info messages will not be logged:
	//
	//     // example: enable logging only for Warn level and above
	//     // (note: ldlog is github.com/launchdarkly/go-sdk-common/v3/ldlog)
	//     config.Logging = ldcomponents.Logging().MinLevel(ldlog.Warn)
	Logging interfaces.LoggingConfigurationFactory

	// Sets whether this client is offline. An offline client will not make any network connections to LaunchDarkly,
	// and will return default values for all feature flags.
	//
	// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/offline-mode#go
	Offline bool

	// Provides configuration of custom service base URIs.
	//
	// Set this field only if you want to specify non-default values for any of the URIs. You may set
	// individual values such as Streaming, or use the helper method ldcomponents.RelayProxyEndpoints().
	//
	// The default behavior, if you do not set any of these values, is that the SDK will connect to
	// the standard endpoints in the LaunchDarkly production service. There are several use cases for
	// changing these values:
	//
	// - You are using the LaunchDarkly Relay Proxy (https://docs.launchdarkly.com/home/advanced/relay-proxy).
	// In this case, call ldcomponents.RelayProxyEndpoints and put its return value into
	// Config.ServiceEndpoints. Note that this is not the same as a regular HTTP proxy, which would
	// be set with ldcomponents.HTTPConfiguration().
	//
	//     config := ld.Config{
	//         ServiceEndpoints: ldcomponents.RelayProxyEndpoints("http://my-relay-host:8080"),
	//     }
	//
	//     // Or, if you want analytics events to be delivered directly to LaunchDarkly rather
	//     // than having them forwarded through the Relay Proxy:
	//     config := ld.Config{
	//         ServiceEndpoints: ldcomponents.RelayProxyEndpoints("http://my-relay-host:8080").
	//             WithoutEventForwarding(),
	//     }
	//
	// - You are connecting to a private instance of LaunchDarkly, rather than the standard production
	// services. In this case, there will be custom base URIs for each service, so you must set
	// Streaming, Polling, and Events to whatever URIs that have been defined for your instance.
	//
	//     config := ld.Config{
	//         ServiceEndpoints: interfaces.ServiceEndpoints{
	//             Streaming: "https://some-subdomain-a.launchdarkly.com",
	//             Polling: "https://some-subdomain-b.launchdarkly.com",
	//             Events: "https://some-subdomain-c.launchdarkly.com",
	//         },
	//     }
	//
	// - You are connecting to a test fixture that simulates the service endpoints. In this case, you
	// may set the base URIs to whatever you want, although the SDK will still set the URI paths to
	// the expected paths for LaunchDarkly services.
	ServiceEndpoints interfaces.ServiceEndpoints

	// Provides configuration of application metadata. See interfaces.ApplicationInfo.
	//
	// Application metadata may be used in LaunchDarkly analytics or other product features, but does not
	// affect feature flag evaluations.
	ApplicationInfo interfaces.ApplicationInfo
}

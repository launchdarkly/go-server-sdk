package ldclient

import (
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// Config exposes advanced configuration options for [LDClient].
//
// All of these settings are optional, so an empty Config struct is always valid. See the description of each
// field for the default behavior if it is not set.
//
// Some of the Config fields are simple types, but others contain configuration builders for subcomponents of
// the SDK. When these are represented by the ComponentConfigurer interface, the actual implementation types
// are provided by corresponding functions in the [ldcomponents] package. For instance, to set the Events
// field to a configuration in which the SDK will flush analytics events every 10 seconds:
//
//	var config ld.Config
//	config.Events = ldcomponents.Events().FlushInterval(time.Second * 10)
//
// The interfaces are defined separately from the built-in component implementations because you could also
// define your own implementation, for custom SDK integrations.
type Config struct {
	// Provides configuration of the SDK's Big Segments feature.
	//
	// "Big Segments" are a specific type of user segments. For more information, read the LaunchDarkly
	// documentation about user segments: https://docs.launchdarkly.com/home/users
	//
	// To enable Big Segments, set this field to the configuration builder that is returned by
	// ldcomponents.BigSegments(), which allows you to specify what database to use as well as other
	// options.
	//
	// If nil, there is no implementation and Big Segments cannot be evaluated. In this case, any flag
	// evaluation that references a Big Segment will behave as if no users are included in any Big
	// Segments, and the EvaluationReason associated with any such flag evaluation will return
	// ldreason.BigSegmentsStoreNotConfigured from its GetBigSegmentsStatus() method.
	//
	//     // example: use Redis, with default properties
	//     import ldredis "github.com/launchdarkly/go-server-sdk-redis-redigo"
	//
	//     config.BigSegmentStore = ldcomponents.BigSegments(ldredis.BigSegmentStore())
	BigSegments subsystems.ComponentConfigurer[subsystems.BigSegmentsConfiguration]

	// Sets the implementation of DataSource for receiving feature flag updates.
	//
	// If Offline is set to true, then DataSource is ignored.
	//
	// The interface type for this field allows you to set it to any of the following:
	//   - ldcomponents.StreamingDataSource(), which enables streaming data and provides a builder to further
	//     configure streaming behavior.
	//   - ldcomponents.PollingDataSource(), which turns off streaming, enables polling, and provides a builder
	//     to further configure polling behavior.
	//   - ldcomponents.ExternalUpdatesOnly(), which turns off all data sources unless an external process is
	//     providing data via a database.
	//   - ldfiledata.DataSource() or ldtestdata.DataSource(), which provide configurable local data sources
	//     for testing.
	//   - Or, a custom component that implements ComponentConfigurer[DataSource].
	//
	//     // example: using streaming mode and setting streaming options
	//     config.DataSource = ldcomponents.StreamingDataSource().InitialReconnectDelay(time.Second)
	//
	//     // example: using polling mode and setting polling options
	//     config.DataSource = ldcomponents.PollingDataSource().PollInterval(time.Minute)
	//
	//     // example: specifying that data will be updated by an external process (such as the Relay Proxy)
	//     config.DataSource = ldcomponents.ExternalUpdatesOnly()
	DataSource subsystems.ComponentConfigurer[subsystems.DataSource]

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
	DataStore subsystems.ComponentConfigurer[subsystems.DataStore]

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
	// The interface type for this field allows you to set it to either:
	//   - ldcomponents.SendEvents(), a configuration builder that allows you to customize event behavior;
	//   - ldcomponents.NoEvents(), which turns off event delivery.
	//
	// If this field is unset/nil, the default is ldcomponents.SendEvents() with no custom options.
	//
	// If Offline is set to true, then event delivery is always off and Events is ignored.
	//
	//     // example: enable events, flush the events every 10 seconds, buffering up to 5000 events
	//     config.Events = ldcomponents.SendEvents().FlushInterval(10 * time.Second).Capacity(5000)
	Events subsystems.ComponentConfigurer[ldevents.EventProcessor]

	// Provides configuration of the SDK's network connection behavior.
	//
	// The interface type used here is implemented by ldcomponents.HTTPConfigurationBuilder, which
	// you can create by calling ldcomponents.HTTPConfiguration(). See that method for an explanation
	// of how to configure the builder. If nil, the default is ldcomponents.HTTPConfiguration() with
	// no custom settings.
	//
	// If Offline is set to true, then HTTP is ignored.
	//
	//     // example: set connection timeout to 8 seconds and use a proxy server
	//     config.HTTP = ldcomponents.HTTPConfiguration().ConnectTimeout(8 * time.Second).ProxyURL(myProxyURL)
	HTTP subsystems.ComponentConfigurer[subsystems.HTTPConfiguration]

	// Provides configuration of the SDK's logging behavior.
	//
	// The interface type used here is implemented by ldcomponents.LoggingConfigurationBuilder, which
	// you can create by calling ldcomponents.Logging(). See that method for an explanation of how to
	// configure the builder. If nil, the default is ldcomponents.Logging() with no custom settings.
	// You can also set this field to ldcomponents.NoLogging() to disable all logging.
	//
	// This example sets the minimum logging level to Warn, so Debug and Info messages will not be logged:
	//
	//     // example: enable logging only for Warn level and above
	//     // (note: ldlog is github.com/launchdarkly/go-sdk-common/v3/ldlog)
	//     config.Logging = ldcomponents.Logging().MinLevel(ldlog.Warn)
	Logging subsystems.ComponentConfigurer[subsystems.LoggingConfiguration]

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

	// Initial set of hooks for the client.
	//
	// Hooks provide entrypoints which allow for observation of SDK functions.
	//
	// LaunchDarkly provides integration packages, and most applications will not
	// need to implement their own hooks.
	Hooks []ldhooks.Hook

	// This field is not stable, and not subject to any backwards compatability guarantees or semantic versioning.
	// It is not suitable for production usage. Do not use it. You have been warned.
	//
	// DataSystem configures how data (e.g. flags, segments) are retrieved by the SDK.
	//
	// Set this field only if you want to specify non-default values for any of the data system configuration,
	// such as defining an alternate data source or setting up a persistent store.
	//
	// Below, the default configuration is described with the relevant config item in parentheses:
	// 1. The SDK will first attempt to fetch all data from LaunchDarkly's global Content Delivery Network (Initializer)
	// 2. It will then establish a streaming connection with LaunchDarkly's realtime Flag Delivery Network (Primary
	//    Synchronizer.)
	// 3. If at any point the connection to the realtime network is interrupted for a short period of time,
	//    the connection will be automatically re-established.
	// 4. If the connection cannot be re-established over a sustained period, the SDK will begin to make periodic
	//    requests to LaunchDarkly's global CDN (Secondary Synchronizer)
	// 5. After a period of time, the SDK will swap back to the realtime Flag Delivery Network if it becomes
	//    available again.
	DataSystem subsystems.ComponentConfigurer[subsystems.DataSystemConfiguration]
}

package ldcomponents

import (
	"fmt"

	ss "github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// DataSystemConfigurationBuilder is a builder for configuring the SDK's data acquisition strategy.
type DataSystemConfigurationBuilder struct {
	storeBuilder          ss.ComponentConfigurer[ss.DataStore]
	storeMode             ss.DataStoreMode
	initializerBuilders   []ss.ComponentConfigurer[ss.DataInitializer]
	synchronizerBuilders  []ss.ComponentConfigurer[ss.DataSynchronizer]
	fdv1FallbackBuilder   ss.ComponentConfigurer[ss.DataSynchronizer]
	overrideSourceBuilder ss.ComponentConfigurer[ss.OverrideSource]
	config                ss.DataSystemConfiguration
}

// Endpoints represents custom endpoints for LaunchDarkly streaming and polling services.
//
// You may specify none, one, or both of these endpoints via WithEndpoints. If an endpoint isn't specified,
// then the default endpoint for that service will be used.
//
// This is a convenience that is identical to individually configuring polling or streaming synchronizer
// BaseURI's using their specific builder functions.
//
// To specify Relay Proxy endpoints, use WithRelayProxyEndpoints.
type Endpoints struct {
	Streaming string
	Polling   string
}

// DataSystemModes provides access to high level strategies for fetching data. The default mode
// is suitable for most use-cases.
type DataSystemModes struct {
	endpoints Endpoints
}

// Default is LaunchDarkly's recommended flag data acquisition strategy. Currently, it operates a
// two-phase method for obtaining data: first, it requests data from LaunchDarkly's global CDN. Then, it initiates
// a streaming connection to LaunchDarkly's Flag Delivery services to receive real-time updates. If
// the streaming connection is interrupted for an extended period of time, the SDK will automatically fall back
// to polling the global CDN for updates.
func (d *DataSystemModes) Default() *DataSystemConfigurationBuilder {
	streaming := StreamingDataSourceV2()
	if d.endpoints.Streaming != "" {
		streaming.BaseURI(d.endpoints.Streaming)
	}

	polling := PollingDataSourceV2()
	fallback := FDv1PollingDataSourceV2()
	if d.endpoints.Polling != "" {
		polling.BaseURI(d.endpoints.Polling)
		fallback.BaseURI(d.endpoints.Polling)
	}

	return d.Custom().
		Initializers(polling.AsInitializer()).
		Synchronizers(streaming, polling).
		FDv1CompatibleSynchronizer(fallback)
}

// Streaming configures the SDK to efficiently streams flag/segment data in the background,
// allowing evaluations to operate on the latest data with no additional latency.
func (d *DataSystemModes) Streaming() *DataSystemConfigurationBuilder {
	streaming := StreamingDataSourceV2()
	if d.endpoints.Streaming != "" {
		streaming.BaseURI(d.endpoints.Streaming)
	}
	fallback := FDv1PollingDataSourceV2()
	if d.endpoints.Polling != "" {
		fallback.BaseURI(d.endpoints.Polling)
	}
	return d.Custom().Synchronizers(streaming).FDv1CompatibleSynchronizer(fallback)
}

// Polling configures the SDK to regularly poll an endpoint for flag/segment data in the background.
// This is less efficient than streaming, but may be necessary in some network environments.
func (d *DataSystemModes) Polling() *DataSystemConfigurationBuilder {
	polling := PollingDataSourceV2()
	fallback := FDv1PollingDataSourceV2()
	if d.endpoints.Polling != "" {
		polling.BaseURI(d.endpoints.Polling)
		fallback.BaseURI(d.endpoints.Polling)
	}
	return d.Custom().Synchronizers(polling).FDv1CompatibleSynchronizer(fallback)
}

// Daemon configures the SDK to read from a persistent store integration that is populated by Relay Proxy
// or other SDKs. The SDK will not connect to LaunchDarkly. In this mode, the SDK never writes to the data store.
func (d *DataSystemModes) Daemon(store ss.ComponentConfigurer[ss.DataStore]) *DataSystemConfigurationBuilder {
	return d.Custom().DataStore(store, ss.DataStoreModeRead)
}

// PersistentStore is similar to Default, with the addition of a
// persistent store integration. Before data has arrived from LaunchDarkly, the SDK is able to
// evaluate flags using data from the persistent store. Once fresh data is available, the SDK
// will no longer read from the persistent store, although it will keep it up-to-date.
func (d *DataSystemModes) PersistentStore(store ss.ComponentConfigurer[ss.DataStore]) *DataSystemConfigurationBuilder {
	return d.Default().DataStore(store, ss.DataStoreModeReadWrite)
}

// Custom returns a builder suitable for creating a custom data acquisition strategy. You may configure
// how the SDK uses a Persistent Store, how the SDK obtains an initial set of data, and how the SDK keeps data
// up-to-date.
func (d *DataSystemModes) Custom() *DataSystemConfigurationBuilder {
	return &DataSystemConfigurationBuilder{}
}

// WithEndpoints configures the data system with custom endpoints for LaunchDarkly's streaming
// and polling synchronizers. This method is not necessary for most use-cases, but can be useful for
// testing or custom network configurations.
//
// Any endpoint that is not specified (empty string) will be treated as the default LaunchDarkly SaaS endpoint
// for that service.
func (d *DataSystemModes) WithEndpoints(endpoints Endpoints) *DataSystemModes {
	if endpoints.Streaming != "" {
		d.endpoints.Streaming = endpoints.Streaming
	}
	if endpoints.Polling != "" {
		d.endpoints.Polling = endpoints.Polling
	}
	return d
}

// WithRelayProxyEndpoints configures the data system with a single endpoint for LaunchDarkly's streaming
// and polling synchronizers. The endpoint should be Relay Proxy's base URI, for example http://localhost:8123.
func (d *DataSystemModes) WithRelayProxyEndpoints(baseURI string) *DataSystemModes {
	return d.WithEndpoints(Endpoints{Streaming: baseURI, Polling: baseURI})
}

// DataSystem provides a high-level selection of the SDK's data acquisition strategy. Use the returned builder to
// select a mode, or to create a custom data acquisition strategy. To use LaunchDarkly's recommended mode, use Default.
func DataSystem() *DataSystemModes {
	return &DataSystemModes{endpoints: Endpoints{
		Streaming: DefaultStreamingBaseURI,
		Polling:   DefaultPollingBaseURI,
	}}
}

// DataStore configures the SDK with an optional data store. The store allows the SDK to serve flag
// values before becoming connected to LaunchDarkly.
func (d *DataSystemConfigurationBuilder) DataStore(store ss.ComponentConfigurer[ss.DataStore],
	storeMode ss.DataStoreMode,
) *DataSystemConfigurationBuilder {
	d.storeBuilder = store
	d.storeMode = storeMode
	return d
}

// Initializers configures the SDK with one or more DataInitializers, which are responsible for fetching
// complete payloads of flag data. The SDK will run the initializers in the order they are specified,
// stopping when one successfully returns data.
func (d *DataSystemConfigurationBuilder) Initializers(
	initializers ...ss.ComponentConfigurer[ss.DataInitializer],
) *DataSystemConfigurationBuilder {
	d.initializerBuilders = initializers
	return d
}

// Synchronizers configures the SDK with an ordered list of synchronizers.
// The SDK tries them in order, falling back to the next synchronizer if one fails.
// When a synchronizer fails and recovery conditions are met, the SDK returns to the first synchronizer.
func (d *DataSystemConfigurationBuilder) Synchronizers(
	synchronizers ...ss.ComponentConfigurer[ss.DataSynchronizer],
) *DataSystemConfigurationBuilder {
	d.synchronizerBuilders = synchronizers
	return d
}

// FDv1CompatibleSynchronizer configures the SDK with a fallback synchronizer that is compatible
// with the Flag Delivery v1 API.
func (d *DataSystemConfigurationBuilder) FDv1CompatibleSynchronizer(
	fallback ss.ComponentConfigurer[ss.DataSynchronizer],
) *DataSystemConfigurationBuilder {
	d.fdv1FallbackBuilder = fallback
	return d
}

// Overrides configures the SDK with an override source, which supplies flag and segment
// definitions that take precedence over data received from LaunchDarkly on a per-key basis.
// Overrides let an operator force one or more flags to a known state on a running client,
// whether or not the client can reach LaunchDarkly; flags not present in the override data
// are unaffected.
//
// The override source is not a data source: it has no effect on the client's initialization
// status or data source status, and configuring it changes nothing until the source
// actually supplies an override.
func (d *DataSystemConfigurationBuilder) Overrides(
	source ss.ComponentConfigurer[ss.OverrideSource],
) *DataSystemConfigurationBuilder {
	d.overrideSourceBuilder = source
	return d
}

// Build creates a DataSystemConfiguration from the configuration provided to the builder.
func (d *DataSystemConfigurationBuilder) Build(
	context ss.ClientContext,
) (ss.DataSystemConfiguration, error) {
	conf := d.config

	if d.storeBuilder != nil {
		store, err := d.storeBuilder.Build(context)
		if err != nil {
			return ss.DataSystemConfiguration{}, err
		}
		conf.Store = store
	}
	conf.StoreMode = d.storeMode
	for i, initializerBuilder := range d.initializerBuilders {
		if initializerBuilder == nil {
			return ss.DataSystemConfiguration{},
				fmt.Errorf("initializer %d is nil", i)
		}
		initializer, err := initializerBuilder.Build(context)
		if err != nil {
			return ss.DataSystemConfiguration{}, err
		}
		conf.Initializers = append(conf.Initializers, initializer)
	}

	// Build synchronizer list
	for i, builder := range d.synchronizerBuilders {
		if builder == nil {
			return ss.DataSystemConfiguration{},
				fmt.Errorf("synchronizer %d is nil", i)
		}

		// Capture builder in closure to avoid loop variable issues
		b := builder
		conf.Synchronizers.SynchronizerBuilders = append(
			conf.Synchronizers.SynchronizerBuilders,
			func() (ss.DataSynchronizer, error) {
				return b.Build(context)
			},
		)
	}
	if d.fdv1FallbackBuilder != nil {
		conf.Synchronizers.FDv1FallbackBuilder = func() (ss.DataSynchronizer, error) {
			return d.fdv1FallbackBuilder.Build(context)
		}
	}
	if d.overrideSourceBuilder != nil {
		overrideSource, err := d.overrideSourceBuilder.Build(context)
		if err != nil {
			return ss.DataSystemConfiguration{}, err
		}
		conf.OverrideSource = overrideSource
	}

	return conf, nil
}

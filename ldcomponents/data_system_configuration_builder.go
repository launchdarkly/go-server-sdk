package ldcomponents

import (
	"errors"
	"fmt"

	ss "github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// DataSystemConfigurationBuilder is a builder for configuring the SDK's data acquisition strategy.
type DataSystemConfigurationBuilder struct {
	storeBuilder         ss.ComponentConfigurer[ss.DataStore]
	storeMode            ss.DataStoreMode
	initializerBuilders  []ss.ComponentConfigurer[ss.DataInitializer]
	primarySyncBuilder   ss.ComponentConfigurer[ss.DataSynchronizer]
	secondarySyncBuilder ss.ComponentConfigurer[ss.DataSynchronizer]
	config               ss.DataSystemConfiguration
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
	if d.endpoints.Polling != "" {
		polling.BaseURI(d.endpoints.Polling)
	}
	return d.Custom().Initializers(polling.AsInitializer()).Synchronizers(streaming, polling)
}

// Streaming configures the SDK to efficiently streams flag/segment data in the background,
// allowing evaluations to operate on the latest data with no additional latency.
func (d *DataSystemModes) Streaming() *DataSystemConfigurationBuilder {
	streaming := StreamingDataSourceV2()
	if d.endpoints.Streaming != "" {
		streaming.BaseURI(d.endpoints.Streaming)
	}
	return d.Custom().Synchronizers(streaming, nil)
}

// Polling configures the SDK to regularly poll an endpoint for flag/segment data in the background.
// This is less efficient than streaming, but may be necessary in some network environments.
func (d *DataSystemModes) Polling() *DataSystemConfigurationBuilder {
	polling := PollingDataSourceV2()
	if d.endpoints.Polling != "" {
		polling.BaseURI(d.endpoints.Polling)
	}
	return d.Custom().Synchronizers(polling, nil)
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
	storeMode ss.DataStoreMode) *DataSystemConfigurationBuilder {
	d.storeBuilder = store
	d.storeMode = storeMode
	return d
}

// Initializers configures the SDK with one or more DataInitializers, which are responsible for fetching
// complete payloads of flag data. The SDK will run the initializers in the order they are specified,
// stopping when one successfully returns data.
func (d *DataSystemConfigurationBuilder) Initializers(
	initializers ...ss.ComponentConfigurer[ss.DataInitializer]) *DataSystemConfigurationBuilder {
	d.initializerBuilders = initializers
	return d
}

// Synchronizers configures the SDK with a primary and secondary synchronizer. The primary is responsible
// for keeping the SDK's data up-to-date, and the SDK will fall back to the secondary in case of a
// primary outage.
func (d *DataSystemConfigurationBuilder) Synchronizers(primary,
	secondary ss.ComponentConfigurer[ss.DataSynchronizer]) *DataSystemConfigurationBuilder {
	d.primarySyncBuilder = primary
	d.secondarySyncBuilder = secondary
	return d
}

// Build creates a DataSystemConfiguration from the configuration provided to the builder.
func (d *DataSystemConfigurationBuilder) Build(
	context ss.ClientContext,
) (ss.DataSystemConfiguration, error) {
	conf := d.config
	if d.secondarySyncBuilder != nil && d.primarySyncBuilder == nil {
		return ss.DataSystemConfiguration{}, errors.New("cannot have a secondary synchronizer without " +
			"a primary synchronizer")
	}
	if d.storeBuilder != nil {
		store, err := d.storeBuilder.Build(context)
		if err != nil {
			return ss.DataSystemConfiguration{}, err
		}
		conf.Store = store
	}
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
	if d.primarySyncBuilder != nil {
		primarySync, err := d.primarySyncBuilder.Build(context)
		if err != nil {
			return ss.DataSystemConfiguration{}, err
		}
		conf.Synchronizers.Primary = primarySync
	}
	if d.secondarySyncBuilder != nil {
		secondarySync, err := d.secondarySyncBuilder.Build(context)
		if err != nil {
			return ss.DataSystemConfiguration{}, err
		}
		conf.Synchronizers.Secondary = secondarySync
	}
	return conf, nil
}

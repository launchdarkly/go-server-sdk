package ldcomponents

import (
	"errors"
	"fmt"
	ss "github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type DataSystemConfigurationBuilder struct {
	storeBuilder         ss.ComponentConfigurer[ss.DataStore]
	storeMode            ss.DataStoreMode
	initializerBuilders  []ss.ComponentConfigurer[ss.DataInitializer]
	primarySyncBuilder   ss.ComponentConfigurer[ss.DataSynchronizer]
	secondarySyncBuilder ss.ComponentConfigurer[ss.DataSynchronizer]
	config               ss.DataSystemConfiguration
}

type DataSystemModes struct{}

// Default is LaunchDarkly's recommended flag data acquisition strategy. Currently, it operates a
// two-phase method for obtaining data: first, it requests data from LaunchDarkly's global CDN. Then, it initiates
// a streaming connection to LaunchDarkly's Flag Delivery services to receive real-time updates. If
// the streaming connection is interrupted for an extended period of time, the SDK will automatically fall back
// to polling the global CDN for updates.
func (d *DataSystemModes) Default() *DataSystemConfigurationBuilder {
	return d.Custom().
		Initializers(PollingDataSourceV2().AsInitializer()).Synchronizers(StreamingDataSourceV2(), PollingDataSourceV2())
}

// Streaming configures the SDK to efficiently streams flag/segment data in the background,
// allowing evaluations to operate on the latest data with no additional latency.
func (d *DataSystemModes) Streaming() *DataSystemConfigurationBuilder {
	return d.Custom().Synchronizers(StreamingDataSourceV2(), nil)
}

// Polling configures the SDK to regularly poll an endpoint for flag/segment data in the background.
// This is less efficient than streaming, but may be necessary in some network environments.
func (d *DataSystemModes) Polling() *DataSystemConfigurationBuilder {
	return d.Custom().Synchronizers(PollingDataSourceV2(), nil)
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
// how the SDK uses a Persistent Store, how the SDK obtains an initial set of data, and how the SDK keeps data up-to-date.
func (d *DataSystemModes) Custom() *DataSystemConfigurationBuilder {
	return &DataSystemConfigurationBuilder{}
}

// DataSystem provides a high-level selection of the SDK's data acquisition strategy. Use the returned builder to select
// a mode, or to create a custom data acquisition strategy. To use LaunchDarkly's recommended mode, use Default.
func DataSystem() *DataSystemModes {
	return &DataSystemModes{}
}

func (d *DataSystemConfigurationBuilder) DataStore(store ss.ComponentConfigurer[ss.DataStore], storeMode ss.DataStoreMode) *DataSystemConfigurationBuilder {
	d.storeBuilder = store
	d.storeMode = storeMode
	return d
}

func (d *DataSystemConfigurationBuilder) Initializers(initializers ...ss.ComponentConfigurer[ss.DataInitializer]) *DataSystemConfigurationBuilder {
	d.initializerBuilders = initializers
	return d
}

func (d *DataSystemConfigurationBuilder) Synchronizers(primary, secondary ss.ComponentConfigurer[ss.DataSynchronizer]) *DataSystemConfigurationBuilder {
	d.primarySyncBuilder = primary
	d.secondarySyncBuilder = secondary
	return d
}

func (d *DataSystemConfigurationBuilder) Build(
	context ss.ClientContext,
) (ss.DataSystemConfiguration, error) {
	conf := d.config
	if d.secondarySyncBuilder != nil && d.primarySyncBuilder == nil {
		return ss.DataSystemConfiguration{}, errors.New("cannot have a secondary synchronizer without a primary synchronizer")
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
			return ss.DataSystemConfiguration{}, fmt.Errorf("initializer %d is nil", i)
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

package ldcomponents

import (
	"errors"
	"fmt"
	ss "github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type DataSystemConfigurationBuilder struct {
	storeBuilder         ss.ComponentConfigurer[ss.DataStore]
	storeMode            ss.StoreMode
	initializerBuilders  []ss.ComponentConfigurer[ss.DataInitializer]
	primarySyncBuilder   ss.ComponentConfigurer[ss.DataSynchronizer]
	secondarySyncBuilder ss.ComponentConfigurer[ss.DataSynchronizer]
	config               ss.DataSystemConfiguration
}

// DataSystem returns a configuration builder that is pre-configured with LaunchDarkly's recommended
// data acquisition strategy. It is equivalent to StreamingMode().
//
// In this mode, the SDK efficiently streams flag/segment data in the background,
// allowing evaluations to operate on the latest data with no additional latency.
func DataSystem() *DataSystemConfigurationBuilder {
	return StreamingMode()
}

// UnconfiguredDataSystem returns a configuration builder with no options set. It is suitable for
// building custom use-cases.
func UnconfiguredDataSystem() *DataSystemConfigurationBuilder {
	return &DataSystemConfigurationBuilder{}
}

// StreamingMode configures the SDK to efficiently streams flag/segment data in the background,
// allowing evaluations to operate on the latest data with no additional latency.
func StreamingMode() *DataSystemConfigurationBuilder {
	return UnconfiguredDataSystem().
		Initializers(PollingDataSourceV2().AsInitializer()).Synchronizers(StreamingDataSourceV2(), PollingDataSourceV2())
}

// PollingMode configures the SDK to regularly poll an endpoint for flag/segment data in the background.
// This is less efficient than streaming, but may be necessary in some network environments.
func PollingMode() *DataSystemConfigurationBuilder {
	return UnconfiguredDataSystem().Synchronizers(PollingDataSourceV2(), nil)
}

// DaemonMode configures the SDK to read from a persistent store integration that is populated by Relay Proxy
// or other SDKs. The SDK will not connect to LaunchDarkly. In this mode, the SDK never writes to the data store.
func DaemonMode(store ss.ComponentConfigurer[ss.DataStore]) *DataSystemConfigurationBuilder {
	return UnconfiguredDataSystem().DataStore(store, ss.StoreModeRead)
}

// PersistentStoreMode is similar to the default DataSystem configuration, with the addition of a
// persistent store integration. Before data has arrived from the streaming connection, the SDK is able to
// evaluate flags using data from the persistent store. Once data has arrived from the streaming connection, the SDK
// will no longer read from the persistent store, although it will keep it up-to-date.
func PersistentStoreMode(store ss.ComponentConfigurer[ss.DataStore]) *DataSystemConfigurationBuilder {
	return StreamingMode().DataStore(store, ss.StoreModeReadWrite)
}

// Offline configures the SDK to evaluate flags using only the default values defined in the application code. No
// outbound connections will be made by the SDK.
func Offline() *DataSystemConfigurationBuilder {
	return UnconfiguredDataSystem().Offline(true)
}

func (d *DataSystemConfigurationBuilder) DataStore(store ss.ComponentConfigurer[ss.DataStore], storeMode ss.StoreMode) *DataSystemConfigurationBuilder {
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

func (d *DataSystemConfigurationBuilder) Offline(offline bool) *DataSystemConfigurationBuilder {
	d.config.Offline = offline
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

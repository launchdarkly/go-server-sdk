package ldcomponents

import (
	"errors"
	"fmt"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type DataSystemConfigurationBuilder struct {
	storeBuilder         subsystems.ComponentConfigurer[subsystems.DataStore]
	storeMode            subsystems.StoreMode
	initializerBuilders  []subsystems.ComponentConfigurer[subsystems.Initializer]
	primarySyncBuilder   subsystems.ComponentConfigurer[subsystems.Synchronizer]
	secondarySyncBuilder subsystems.ComponentConfigurer[subsystems.Synchronizer]
	config               subsystems.DataSystemConfiguration
}

func DataSystem() *DataSystemConfigurationBuilder {
	return &DataSystemConfigurationBuilder{}
}

func DaemonModeV2(store subsystems.ComponentConfigurer[subsystems.DataStore]) *DataSystemConfigurationBuilder {
	return DataSystem().DataStore(store, subsystems.StoreModeRead)
}

func PersistentStoreV2(store subsystems.ComponentConfigurer[subsystems.DataStore]) *DataSystemConfigurationBuilder {
	return StreamingDataSourceV2().DataStore(store, subsystems.StoreModeReadWrite)
}

func PollingDataSourceV2() *DataSystemConfigurationBuilder {
	return DataSystem().Synchronizers(ldcomponents.PollingDataSource(), nil)
}

func StreamingDataSourceV2() *DataSystemConfigurationBuilder {
	return DataSystem().Initializers(ldcomponents.PollingInitializer()).Synchronizers(ldcomponents.StreamingDataSource(), ldcomponents.PollingDataSource())
}

func (d *DataSystemConfigurationBuilder) DataStore(store subsystems.ComponentConfigurer[subsystems.DataStore], storeMode subsystems.StoreMode) *DataSystemConfigurationBuilder {
	d.storeBuilder = store
	d.storeMode = storeMode
	return d
}

func (d *DataSystemConfigurationBuilder) Initializers(initializers ...subsystems.ComponentConfigurer[subsystems.Initializer]) *DataSystemConfigurationBuilder {
	d.initializerBuilders = initializers
	return d
}

func (d *DataSystemConfigurationBuilder) Synchronizers(primary, secondary subsystems.ComponentConfigurer[subsystems.Synchronizer]) *DataSystemConfigurationBuilder {
	d.primarySyncBuilder = primary
	d.secondarySyncBuilder = secondary
	return d
}

func (d *DataSystemConfigurationBuilder) Build(
	context subsystems.ClientContext,
) (subsystems.DataSystemConfiguration, error) {
	conf := d.config
	if d.secondarySyncBuilder != nil && d.primarySyncBuilder == nil {
		return subsystems.DataSystemConfiguration{}, errors.New("cannot have a secondary synchronizer without a primary synchronizer")
	}
	if d.storeBuilder != nil {
		store, err := d.storeBuilder.Build(context)
		if err != nil {
			return subsystems.DataSystemConfiguration{}, err
		}
		conf.Store = store
	}
	for i, initializerBuilder := range d.initializerBuilders {
		if initializerBuilder == nil {
			return subsystems.DataSystemConfiguration{}, fmt.Errorf("initializer %d is nil", i)
		}
		initializer, err := initializerBuilder.Build(context)
		if err != nil {
			return subsystems.DataSystemConfiguration{}, err
		}
		conf.Initializers = append(conf.Initializers, initializer)
	}
	if d.primarySyncBuilder != nil {
		primarySync, err := d.primarySyncBuilder.Build(context)
		if err != nil {
			return subsystems.DataSystemConfiguration{}, err
		}
		conf.Synchronizers.Primary = primarySync
	}
	if d.secondarySyncBuilder != nil {
		secondarySync, err := d.secondarySyncBuilder.Build(context)
		if err != nil {
			return subsystems.DataSystemConfiguration{}, err
		}
		conf.Synchronizers.Secondary = secondarySync
	}
	return conf, nil
}

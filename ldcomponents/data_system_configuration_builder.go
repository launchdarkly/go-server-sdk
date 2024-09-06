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

func DataSystem() *DataSystemConfigurationBuilder {
	return &DataSystemConfigurationBuilder{}
}

func DaemonModeV2(store ss.ComponentConfigurer[ss.DataStore]) *DataSystemConfigurationBuilder {
	return DataSystem().DataStore(store, ss.StoreModeRead)
}

//func PersistentStoreV2(store ss.ComponentConfigurer[ss.DataStore]) *DataSystemConfigurationBuilder {
//	return StreamingDataSourceV2().DataStore(store, ss.StoreModeReadWrite)
//}

//func PollingDataSourceV2() *DataSystemConfigurationBuilder {
//	return DataSystem().Synchronizers(PollingDataSource().V2(), nil)
//}
//
//func StreamingDataSourceV2() *DataSystemConfigurationBuilder {
//	return DataSystem().Initializers(PollingDataSource().V2()).Synchronizers(StreamingDataSource().V2(), PollingDataSource().V2())
//}

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

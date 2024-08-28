package ldcomponents

import (
	"errors"
	"fmt"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type DataSystemConfigurationBuilder struct {
	storeBuilder         subsystems.ComponentConfigurer[subsystems.DataStore]
	initializerBuilders  []subsystems.ComponentConfigurer[subsystems.Initializer]
	primarySyncBuilder   subsystems.ComponentConfigurer[subsystems.Synchronizer]
	secondarySyncBuilder subsystems.ComponentConfigurer[subsystems.Synchronizer]
	config               subsystems.DataSystemConfiguration
}

func DataSystem() *DataSystemConfigurationBuilder {
	return &DataSystemConfigurationBuilder{
		primarySyncBuilder:   ldcomponents.StreamingDataSource(),
		secondarySyncBuilder: ldcomponents.PollingDataSource(),
		storeBuilder:         nil, // in-memory only
		initializerBuilders:  []subsystems.ComponentConfigurer[subsystems.Initializer]{ldcomponents.PollingInitializer()},
	}
}

func (d *DataSystemConfigurationBuilder) Store(store subsystems.ComponentConfigurer[subsystems.DataStore]) *DataSystemConfigurationBuilder {
	d.storeBuilder = store
	return d
}

func (d *DataSystemConfigurationBuilder) Initializers(initializers ...subsystems.ComponentConfigurer[subsystems.Initializer]) *DataSystemConfigurationBuilder {
	d.initializerBuilders = initializers
	return d
}

func (d *DataSystemConfigurationBuilder) PrimarySynchronizer(sync subsystems.ComponentConfigurer[subsystems.Synchronizer]) *DataSystemConfigurationBuilder {
	d.primarySyncBuilder = sync
	return d
}

func (d *DataSystemConfigurationBuilder) SecondarySynchronizer(sync subsystems.ComponentConfigurer[subsystems.Synchronizer]) *DataSystemConfigurationBuilder {
	d.secondarySyncBuilder = sync
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

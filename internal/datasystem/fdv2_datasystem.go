package datasystem

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type FDv2 struct {
}

func (F FDv2) DataSourceStatusBroadcaster() *internal.Broadcaster[interfaces.DataSourceStatus] {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) DataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) DataStoreStatusBroadcaster() *internal.Broadcaster[interfaces.DataStoreStatus] {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) DataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) FlagChangeEventBroadcaster() *internal.Broadcaster[interfaces.FlagChangeEvent] {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) Offline() bool {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) Start(closeWhenReady chan struct{}) {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) Stop() error {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) Store() subsystems.ReadOnlyStore {
	//TODO implement me
	panic("implement me")
}

func (F FDv2) DataStatus() DataStatus {
	//TODO implement me
	panic("implement me")
}

func NewFDv2(loggers ldlog.Loggers, configurer subsystems.ComponentConfigurer[subsystems.DataSystemConfiguration]) (*FDv2, error) {
	return &FDv2{}, nil
}

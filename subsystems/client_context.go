package subsystems

import (
	"net/http"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

// ClientContext provides context information from LDClient when creating other components.
//
// This is passed as a parameter to the factory methods for implementations of DataStore, DataSource,
// etc. The actual implementation type may contain other properties that are only relevant to the built-in
// SDK components and are therefore not part of the public interface; this allows the SDK to add its own
// context information as needed without disturbing the public API. However, for test purposes you may use
// the simple struct type BasicClientContext.
type ClientContext interface {
	// GetSDKKey returns the configured SDK key.
	GetSDKKey() string

	// GetApplicationInfo returns the configuration for application metadata.
	GetApplicationInfo() interfaces.ApplicationInfo

	// GetHTTP returns the configured HTTPConfiguration.
	GetHTTP() HTTPConfiguration

	// GetLogging returns the configured LoggingConfiguration.
	GetLogging() LoggingConfiguration

	// GetOffline returns true if the client was configured to be completely offline.
	GetOffline() bool

	// GetServiceEndpoints returns the configuration for service URIs.
	GetServiceEndpoints() interfaces.ServiceEndpoints

	// GetDataSourceUpdateSink returns the component that DataSource implementations use to deliver
	// data and status updates to the SDK.
	//
	// This component is only available when the SDK is creating a DataSource. Otherwise the method
	// returns nil.
	GetDataSourceUpdateSink() DataSourceUpdateSink

	// GetDataStoreUpdateSink returns the component that DataSource implementations use to deliver
	// data store status updates to the SDK.
	//
	// This component is only available when the SDK is creating a DataStore. Otherwise the method
	// returns nil.
	GetDataStoreUpdateSink() DataStoreUpdateSink

	// GetDataDestination is a FDV2 method, do not use. Not subject to semantic versioning.
	// This method is a replacement for GetDataSourceUpdateSink when the SDK is in FDv2 mode.
	GetDataDestination() DataDestination

	// GetDataSourceStatusReporter is a FDV2 method, do not use. Not subject to semantic versioning.
	// This method is a replacement for GetDataSourceUpdateSink when the SDK is in FDv2 mode.
	GetDataSourceStatusReporter() DataSourceStatusReporter
}

// BasicClientContext is the basic implementation of the ClientContext interface, not including any
// private fields that the SDK may use for implementation details.
type BasicClientContext struct {
	SDKKey                   string
	ApplicationInfo          interfaces.ApplicationInfo
	HTTP                     HTTPConfiguration
	Logging                  LoggingConfiguration
	Offline                  bool
	ServiceEndpoints         interfaces.ServiceEndpoints
	DataSourceUpdateSink     DataSourceUpdateSink
	DataStoreUpdateSink      DataStoreUpdateSink
	DataDestination          DataDestination
	DataSourceStatusReporter DataSourceStatusReporter
}

func (b BasicClientContext) GetSDKKey() string { return b.SDKKey } //nolint:revive

func (b BasicClientContext) GetApplicationInfo() interfaces.ApplicationInfo { return b.ApplicationInfo } //nolint:revive

func (b BasicClientContext) GetHTTP() HTTPConfiguration { //nolint:revive
	ret := b.HTTP
	if ret.CreateHTTPClient == nil {
		ret.CreateHTTPClient = func() *http.Client {
			client := *http.DefaultClient
			return &client
		}
	}
	return ret
}

func (b BasicClientContext) GetLogging() LoggingConfiguration { return b.Logging } //nolint:revive

func (b BasicClientContext) GetOffline() bool { return b.Offline } //nolint:revive

func (b BasicClientContext) GetServiceEndpoints() interfaces.ServiceEndpoints { //nolint:revive
	return b.ServiceEndpoints
}

func (b BasicClientContext) GetDataSourceUpdateSink() DataSourceUpdateSink { //nolint:revive
	return b.DataSourceUpdateSink
}

func (b BasicClientContext) GetDataStoreUpdateSink() DataStoreUpdateSink { //nolint:revive
	return b.DataStoreUpdateSink
}

func (b BasicClientContext) GetDataDestination() DataDestination { //nolint:revive
	return b.DataDestination
}

func (b BasicClientContext) GetDataSourceStatusReporter() DataSourceStatusReporter { //nolint:revive
	return b.DataSourceStatusReporter
}

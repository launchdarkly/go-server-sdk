package ldcomponents

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

type nullDataSourceFactory struct{}

// ExternalUpdatesOnly returns a configuration object that disables a direct connection with LaunchDarkly
// for feature flag updates.
//
// Storing this in LDConfig.DataSource causes the SDK not to retrieve feature flag data from LaunchDarkly,
// regardless of any other configuration. This is normally done if you are using the Relay Proxy
// (https://docs.launchdarkly.com/home/relay-proxy) in "daemon mode", where an external process-- the
// Relay Proxy-- connects to LaunchDarkly and populates a persistent data store with the feature flag data.
// The data store could also be populated by another process that is running the LaunchDarkly SDK. If there
// is no external process updating the data store, then the SDK will not have any feature flag data and
// will return application default values only.
//
//	config := ld.Config{
//	    DataSource: ldcomponents.ExternalUpdatesOnly(),
//	}
func ExternalUpdatesOnly() subsystems.ComponentConfigurer[subsystems.DataSource] {
	return nullDataSourceFactory{}
}

// DataSourceFactory implementation
func (f nullDataSourceFactory) Build(
	context subsystems.ClientContext,
) (subsystems.DataSource, error) {
	context.GetLogging().Loggers.Info("LaunchDarkly client will not connect to Launchdarkly for feature flag data")
	if context.GetDataSourceUpdateSink() != nil {
		context.GetDataSourceUpdateSink().UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
	}
	return datasource.NewNullDataSource(), nil
}

// DiagnosticDescription implementation
func (f nullDataSourceFactory) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	// This information is only used for diagnostic events, and if we're able to send diagnostic events,
	// then by definition we're not completely offline so we must be using daemon mode.
	return ldvalue.ObjectBuild().
		SetBool("usingRelayDaemon", true).
		Build()
}

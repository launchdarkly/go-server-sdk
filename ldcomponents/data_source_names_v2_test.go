package ldcomponents

import (
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func describeBuilt(t *testing.T, cc subsystems.ComponentConfigurer[subsystems.DataSynchronizer]) (string, interfaces.DataSourceDescriptor) {
	component, err := cc.Build(makeTestContextWithBaseURIs("base"))
	require.NoError(t, err)
	return component.Name(), subsystems.DescribeDataSource(component)
}

func TestFDv2DataSourceNames(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		name, d := describeBuilt(t, StreamingDataSourceV2())
		assert.Equal(t, "streaming", name)
		assert.Equal(t, interfaces.DataSourceDescriptor{
			Protocol: interfaces.DataSourceProtocolFDv2, Transport: interfaces.DataSourceTransportStreaming, Name: name}, d)

		name, d = describeBuilt(t, PollingDataSourceV2())
		assert.Equal(t, "polling", name)
		assert.Equal(t, interfaces.DataSourceDescriptor{
			Protocol: interfaces.DataSourceProtocolFDv2, Transport: interfaces.DataSourceTransportPolling, Name: name}, d)

		name, d = describeBuilt(t, FDv1PollingDataSourceV2())
		assert.Equal(t, "fdv1_polling", name)
		assert.Equal(t, interfaces.DataSourceDescriptor{
			Protocol: interfaces.DataSourceProtocolFDv1, Transport: interfaces.DataSourceTransportPolling, Name: name}, d)
	})

	t.Run("configured names replace the default in Name and Describe", func(t *testing.T) {
		name, d := describeBuilt(t, StreamingDataSourceV2().Name("launchdarkly-stream"))
		assert.Equal(t, "launchdarkly-stream", name)
		assert.Equal(t, "launchdarkly-stream", d.Name)
		assert.Equal(t, interfaces.DataSourceTransportStreaming, d.Transport)

		name, d = describeBuilt(t, PollingDataSourceV2().Name("relay-poll"))
		assert.Equal(t, "relay-poll", name)
		assert.Equal(t, "relay-poll", d.Name)
		assert.Equal(t, interfaces.DataSourceProtocolFDv2, d.Protocol)

		name, d = describeBuilt(t, FDv1PollingDataSourceV2().Name("relay-fdv1-poll"))
		assert.Equal(t, "relay-fdv1-poll", name)
		assert.Equal(t, interfaces.DataSourceProtocolFDv1, d.Protocol)
	})

	t.Run("an empty name keeps the default", func(t *testing.T) {
		name, _ := describeBuilt(t, PollingDataSourceV2().Name(""))
		assert.Equal(t, "polling", name)
	})

	t.Run("a named polling builder used as an initializer keeps its name", func(t *testing.T) {
		initializer, err := PollingDataSourceV2().Name("relay-poll-init").AsInitializer().Build(makeTestContextWithBaseURIs("base"))
		require.NoError(t, err)
		assert.Equal(t, "relay-poll-init", initializer.Name())
		assert.Equal(t, "relay-poll-init", subsystems.DescribeDataSource(initializer).Name)
	})
}

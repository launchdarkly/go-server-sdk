package interfaces

// DataSourceProtocol identifies the flag delivery protocol that a data source component uses.
type DataSourceProtocol string

const (
	// DataSourceProtocolFDv1 is the original flag delivery protocol.
	DataSourceProtocolFDv1 DataSourceProtocol = "fdv1"
	// DataSourceProtocolFDv2 is the flag delivery protocol used by the data system.
	DataSourceProtocolFDv2 DataSourceProtocol = "fdv2"
)

// DataSourceTransport identifies how a data source component obtains data.
type DataSourceTransport string

const (
	// DataSourceTransportStreaming means the component holds a streaming connection open.
	DataSourceTransportStreaming DataSourceTransport = "streaming"
	// DataSourceTransportPolling means the component makes periodic requests.
	DataSourceTransportPolling DataSourceTransport = "polling"
	// DataSourceTransportFile means the component reads data from local files.
	DataSourceTransportFile DataSourceTransport = "file"
)

// DataSourceDescriptor identifies a data source component: an FDv1 data source, or an FDv2
// initializer or synchronizer. The SDK attaches it to data source status changes and to
// initializer and synchronizer lifecycle events so that observers can tell which component
// produced them.
//
// Any field may be empty when the component does not report it. A custom component that does not
// implement DataSourceDescriber is described by its name only.
type DataSourceDescriptor struct {
	// Protocol is the flag delivery protocol, if known.
	Protocol DataSourceProtocol
	// Transport is how the component obtains data, if known.
	Transport DataSourceTransport
	// Name is the component's name. Built-in components have a default name, for example
	// "StreamingDataSourceV2", which the configuration builders let an application replace so that
	// two components of the same type can be told apart, such as a polling initializer that reads from
	// LaunchDarkly and one that reads from a Relay Proxy.
	Name string
}

// IsDefined returns true if any field of the descriptor is set.
func (d DataSourceDescriptor) IsDefined() bool {
	return d.Protocol != "" || d.Transport != "" || d.Name != ""
}

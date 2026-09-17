package subsystems

import "github.com/launchdarkly/go-server-sdk/v7/interfaces"

// DataSourceDescriber is an optional interface for data source components. A DataSource,
// DataInitializer, or DataSynchronizer that implements it tells the SDK which protocol and
// transport it uses. The SDK reports the descriptor with data source status changes and
// lifecycle events.
type DataSourceDescriber interface {
	// Describe returns the descriptor for this component.
	Describe() interfaces.DataSourceDescriptor
}

type namedComponent interface {
	Name() string
}

// DescribeDataSource returns the descriptor for a data source component. It uses Describe when the
// component implements DataSourceDescriber, falls back to the component's Name, and returns an
// empty descriptor for a component that has neither.
func DescribeDataSource(component any) interfaces.DataSourceDescriptor {
	if describer, ok := component.(DataSourceDescriber); ok {
		return describer.Describe()
	}
	if named, ok := component.(namedComponent); ok {
		return interfaces.DataSourceDescriptor{Name: named.Name()}
	}
	return interfaces.DataSourceDescriptor{}
}

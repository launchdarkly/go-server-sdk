package ldhooks

import "github.com/launchdarkly/go-server-sdk/v7/interfaces"

// DataSourceStatusContext contains the information passed to the DataSourceStatusChanged handler.
//
// The SDK creates a new DataSourceStatusContext each time the data source state changes or the
// data source reports a new error.
type DataSourceStatusContext struct {
	previous   interfaces.DataSourceStatus
	current    interfaces.DataSourceStatus
	dataSource interfaces.DataSourceDescriptor
}

// NewDataSourceStatusContext creates a DataSourceStatusContext. This is called by the SDK.
func NewDataSourceStatusContext(
	previous, current interfaces.DataSourceStatus,
	dataSource interfaces.DataSourceDescriptor,
) DataSourceStatusContext {
	return DataSourceStatusContext{previous: previous, current: current, dataSource: dataSource}
}

// Previous returns the data source status before the change.
//
// The State of the previous status is empty when the SDK reports the first status.
func (c DataSourceStatusContext) Previous() interfaces.DataSourceStatus {
	return c.previous
}

// Current returns the data source status after the change.
func (c DataSourceStatusContext) Current() interfaces.DataSourceStatus {
	return c.current
}

// DataSource identifies the data source component that produced the status. Fields are empty when
// the component does not report them.
func (c DataSourceStatusContext) DataSource() interfaces.DataSourceDescriptor {
	return c.dataSource
}

package internal

import (
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// DataSourceStatusObserver receives data source lifecycle events in-band, at the point where the
// data system produces them. Unlike DataSourceStatusProvider listeners, which receive statuses on a
// channel, an observer runs synchronously.
type DataSourceStatusObserver interface {
	// OnDataSourceStatusChanged is called after the data source status changed. source identifies
	// the component that produced the status; it may be empty.
	OnDataSourceStatusChanged(previous, current interfaces.DataSourceStatus, source interfaces.DataSourceDescriptor)

	// OnInitializerCompleted is called after each attempt to obtain initial data from an initializer.
	OnInitializerCompleted(initializerContext ldhooks.InitializerContext)

	// OnSynchronizerChanged is called when a synchronizer starts or when none remain.
	OnSynchronizerChanged(changeContext ldhooks.SynchronizerChangeContext)

	// OnInitializationCompleted is called once, when the data system first has data or gives up.
	OnInitializationCompleted(initializationContext ldhooks.InitializationContext)
}

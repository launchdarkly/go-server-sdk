package internal

import (
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// ClientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type ClientContextImpl struct {
	subsystems.BasicClientContext
	// Used internally to share a diagnosticsManager instance between components.
	DiagnosticsManager *ldevents.DiagnosticsManager
	// EventMetrics, if non-nil, receives event processing metrics from the standard event processor.
	// The SDK sets this when a registered hook implements an event delivery handler.
	EventMetrics ldevents.EventMetrics
	// DataSourceStatusObserver, if non-nil, receives data source status changes in-band. The SDK sets
	// this when a registered hook implements DataSourceStatusHandler.
	DataSourceStatusObserver DataSourceStatusObserver
}

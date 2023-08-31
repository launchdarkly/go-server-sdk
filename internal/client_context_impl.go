package internal

import (
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// ClientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type ClientContextImpl struct {
	subsystems.BasicClientContext
	// Used internally to share a diagnosticsManager instance between components.
	DiagnosticsManager *ldevents.DiagnosticsManager
}

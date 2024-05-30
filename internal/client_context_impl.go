package internal

import (
	ldevents "github.com/launchdarkly/go-server-sdk/ldevents/v4"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// ClientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type ClientContextImpl struct {
	subsystems.BasicClientContext
	// Used internally to share a diagnosticsManager instance between components.
	DiagnosticsManager *ldevents.DiagnosticsManager
}

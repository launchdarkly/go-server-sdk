package internal

import (
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// ClientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type ClientContextImpl struct {
	subsystems.BasicClientContext
	// Used internally to share a diagnosticsManager instance between components.
	DiagnosticsManager *ldevents.DiagnosticsManager
	// True if components that deserialize flags and segments release their redundant clause value
	// lists. This is the value of Config.ReleaseClauseValues.
	ReleaseClauseValues bool
}

// DataKindDeserializeOptions returns the options that components use to deserialize flags and
// segments. If the context is not a ClientContextImpl, it returns the default options. The context can
// be a pointer or a value, because the FDv2 data system gives a copy of the ClientContextImpl value to
// its components.
func DataKindDeserializeOptions(context subsystems.ClientContext) datakinds.DeserializeOptions {
	switch cci := context.(type) {
	case *ClientContextImpl:
		return datakinds.NewDeserializeOptions(cci.ReleaseClauseValues)
	case ClientContextImpl:
		return datakinds.NewDeserializeOptions(cci.ReleaseClauseValues)
	default:
		return datakinds.DeserializeOptions{}
	}
}

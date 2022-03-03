package internal

import (
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
)

// ClientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type ClientContextImpl struct {
	basic   interfaces.BasicConfiguration
	http    interfaces.HTTPConfiguration
	logging interfaces.LoggingConfiguration
	// Used internally to share a diagnosticsManager instance between components.
	DiagnosticsManager *ldevents.DiagnosticsManager
}

// NewClientContextImpl creates the SDK's standard implementation of interfaces.ClientContext.
func NewClientContextImpl(
	basic interfaces.BasicConfiguration,
	http interfaces.HTTPConfiguration,
	logging interfaces.LoggingConfiguration,
) *ClientContextImpl {
	return &ClientContextImpl{
		basic,
		http,
		logging,
		nil,
	}
}

func (c *ClientContextImpl) GetBasic() interfaces.BasicConfiguration { //nolint:revive
	return c.basic
}

func (c *ClientContextImpl) GetHTTP() interfaces.HTTPConfiguration { //nolint:revive
	return c.http
}

func (c *ClientContextImpl) GetLogging() interfaces.LoggingConfiguration { //nolint:revive
	return c.logging
}

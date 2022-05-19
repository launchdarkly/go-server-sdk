package internal

import (
	"net/http"

	ldevents "github.com/launchdarkly/go-sdk-events/v2"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// ClientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type ClientContextImpl struct {
	basic   subsystems.BasicConfiguration
	http    subsystems.HTTPConfiguration
	logging subsystems.LoggingConfiguration
	// Used internally to share a diagnosticsManager instance between components.
	DiagnosticsManager *ldevents.DiagnosticsManager
}

// NewClientContextImpl creates the SDK's standard implementation of interfaces.ClientContext.
func NewClientContextImpl(
	basic subsystems.BasicConfiguration,
	httpConfig subsystems.HTTPConfiguration,
	logging subsystems.LoggingConfiguration,
) *ClientContextImpl {
	if httpConfig.CreateHTTPClient == nil {
		httpConfig.CreateHTTPClient = func() *http.Client {
			client := *http.DefaultClient
			return &client
		}
	}
	return &ClientContextImpl{
		basic,
		httpConfig,
		logging,
		nil,
	}
}

func (c *ClientContextImpl) GetBasic() subsystems.BasicConfiguration { //nolint:revive
	return c.basic
}

func (c *ClientContextImpl) GetHTTP() subsystems.HTTPConfiguration { //nolint:revive
	return c.http
}

func (c *ClientContextImpl) GetLogging() subsystems.LoggingConfiguration { //nolint:revive
	return c.logging
}

package internal

import (
	"net/http"

	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// clientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type clientContextImpl struct {
	sdkKey            string
	logging           interfaces.LoggingConfiguration
	httpHeaders       http.Header
	httpClientFactory func() *http.Client
	offline           bool
	// Used internally to share a diagnosticsManager instance between components.
	diagnosticsManager *ldevents.DiagnosticsManager
}

// NewClientContextImpl creates the SDK's standard implementation of interfaces.ClientContext.
func NewClientContextImpl(
	sdkKey string,
	logging interfaces.LoggingConfiguration,
	httpHeaders http.Header,
	httpClientFactory func() *http.Client,
	offline bool,
	diagnosticsManager *ldevents.DiagnosticsManager,
) interfaces.ClientContext {
	return &clientContextImpl{sdkKey, logging, httpHeaders, httpClientFactory, offline, diagnosticsManager}
}

func (c *clientContextImpl) GetSDKKey() string {
	return c.sdkKey
}

func (c *clientContextImpl) GetLogging() interfaces.LoggingConfiguration {
	return c.logging
}

func (c *clientContextImpl) GetDefaultHTTPHeaders() http.Header {
	return c.httpHeaders
}

func (c *clientContextImpl) IsOffline() bool {
	return c.offline
}

func (c *clientContextImpl) CreateHTTPClient() *http.Client {
	if c.httpClientFactory == nil {
		client := NewHTTPClient(defaultHTTPTimeout)
		return &client
	}
	return c.httpClientFactory()
}

// This method is accessed by components like StreamProcessor by checking for a private interface.
func (c *clientContextImpl) GetDiagnosticsManager() *ldevents.DiagnosticsManager {
	return c.diagnosticsManager
}

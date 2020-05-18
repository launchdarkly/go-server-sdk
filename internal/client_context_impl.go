package internal

import (
	"net/http"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
)

// ClientContextImpl is the SDK's standard implementation of interfaces.ClientContext.
type ClientContextImpl struct {
	sdkKey            string
	loggers           ldlog.Loggers
	httpHeaders       http.Header
	httpClientFactory func() *http.Client
	offline           bool
	// Used internally to share a diagnosticsManager instance between components.
	diagnosticsManager *ldevents.DiagnosticsManager
}

// NewClientContextImpl creates the SDK's standard implementation of interfaces.ClientContext.
func NewClientContextImpl(
	sdkKey string,
	loggers ldlog.Loggers,
	httpHeaders http.Header,
	httpClientFactory func() *http.Client,
	offline bool,
	diagnosticsManager *ldevents.DiagnosticsManager,
) *ClientContextImpl {
	return &ClientContextImpl{sdkKey, loggers, httpHeaders, httpClientFactory, offline, diagnosticsManager}
}

func (c *ClientContextImpl) GetSDKKey() string { //nolint:golint // no doc comment for standard interface method
	return c.sdkKey
}

func (c *ClientContextImpl) GetLoggers() ldlog.Loggers { //nolint:golint // no doc comment for standard interface method
	return c.loggers
}

func (c *ClientContextImpl) GetDefaultHTTPHeaders() http.Header { //nolint:golint // no doc comment for standard interface method
	return c.httpHeaders
}

func (c *ClientContextImpl) IsOffline() bool { //nolint:golint // no doc comment for standard interface method
	return c.offline
}

func (c *ClientContextImpl) CreateHTTPClient() *http.Client { //nolint:golint // no doc comment for standard interface method
	if c.httpClientFactory == nil {
		client := NewHTTPClient(defaultHTTPTimeout)
		return &client
	}
	return c.httpClientFactory()
}

// This method is accessed by components like StreamProcessor by checking for a private interface.
func (c *ClientContextImpl) GetDiagnosticsManager() *ldevents.DiagnosticsManager {
	return c.diagnosticsManager
}

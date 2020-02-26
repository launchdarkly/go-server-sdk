package ldclient

import (
	"net/http"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldevents"
)

// Internal implementation of interfaces.ClientContext.
type clientContextImpl struct {
	sdkKey            string
	loggers           ldlog.Loggers
	httpHeaders       http.Header
	httpClientFactory func() *http.Client
	// Used internally to share a diagnosticsManager instance between components.
	diagnosticsManager *ldevents.DiagnosticsManager
}

// Components that can use a DiagnosticsManager should check if the context implements this interface
type hasDiagnosticsManager interface {
	GetDiagnosticsManager() *ldevents.DiagnosticsManager
}

func (c *clientContextImpl) GetSDKKey() string {
	return c.sdkKey
}

func (c *clientContextImpl) GetLoggers() ldlog.Loggers {
	return c.loggers
}

func (c *clientContextImpl) GetDefaultHTTPHeaders() http.Header {
	return c.httpHeaders
}

func (c *clientContextImpl) CreateHTTPClient() *http.Client {
	if c.httpClientFactory == nil {
		return DefaultConfig.newHTTPClient()
	}
	return c.httpClientFactory()
}

func (c *clientContextImpl) GetDiagnosticsManager() *ldevents.DiagnosticsManager {
	return c.diagnosticsManager
}

func newClientContextImpl(sdkKey string, config Config, diagnosticsManager *ldevents.DiagnosticsManager) *clientContextImpl {
	headers := make(http.Header)
	headers.Set("Authorization", sdkKey)
	headers.Set("User-Agent", config.UserAgent)
	if config.WrapperName != "" {
		w := config.WrapperName
		if config.WrapperVersion != "" {
			w = w + "/" + config.WrapperVersion
		}
		headers.Add("X-LaunchDarkly-Wrapper", w)
	}
	return &clientContextImpl{
		sdkKey:             sdkKey,
		loggers:            config.Loggers,
		httpHeaders:        headers,
		httpClientFactory:  config.newHTTPClient,
		diagnosticsManager: diagnosticsManager,
	}
}

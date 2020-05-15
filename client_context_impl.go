package ldclient

import (
	"net/http"

	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

// Internal implementation of interfaces.ClientContext.
type clientContextImpl struct {
	sdkKey            string
	logging           interfaces.LoggingConfiguration
	httpHeaders       http.Header
	httpClientFactory func() *http.Client
	// Used internally to share a diagnosticsManager instance between components.
	diagnosticsManager *ldevents.DiagnosticsManager
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

func (c *clientContextImpl) CreateHTTPClient() *http.Client {
	if c.httpClientFactory == nil {
		return Config{}.newHTTPClient()
	}
	return c.httpClientFactory()
}

// This method is accessed by components like StreamProcessor by checking for a private interface.
func (c *clientContextImpl) GetDiagnosticsManager() *ldevents.DiagnosticsManager {
	return c.diagnosticsManager
}

func newClientContextImpl(sdkKey string, config Config, httpClientFactory func() *http.Client, diagnosticsManager *ldevents.DiagnosticsManager) *clientContextImpl {
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
	var logging interfaces.LoggingConfiguration
	if config.Logging == nil {
		logging = ldcomponents.Logging().CreateLoggingConfiguration()
	} else {
		logging = config.Logging.CreateLoggingConfiguration()
	}
	return &clientContextImpl{
		sdkKey:             sdkKey,
		logging:            logging,
		httpHeaders:        headers,
		httpClientFactory:  httpClientFactory,
		diagnosticsManager: diagnosticsManager,
	}
}

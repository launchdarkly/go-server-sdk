package interfaces

import (
	"net/http"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

// ClientContext provides context information from LDClient when creating other components.
//
// This is passed as a parameter to the factory methods for implementations of DataStore, DataSource,
// etc. The actual implementation type may contain other properties that are only relevant to the built-in
// SDK components and are therefore not part of the public interface; this allows the SDK to add its own
// context information as needed without disturbing the public API.
type ClientContext interface {
	// GetSDKKey returns the configured SDK key.
	GetSDKKey() string
	// GetDefaultHTTPHeaders returns the headers that should be included in all HTTP requests from the client
	// to LaunchDarkly services, based on the current configuration.
	GetDefaultHTTPHeaders() http.Header
	// CreateHTTPClient creates an HTTP client instance based on the current configuration.
	CreateHTTPClient() *http.Client
	// GetLogging returns the configured LoggingConfiguration.
	GetLogging() LoggingConfiguration
	// IsOffline returns true if the client was configured to be completely offline.
	IsOffline() bool
}

type basicClientContext struct {
	sdkKey            string
	headers           http.Header
	httpClientFactory func() *http.Client
	logging           LoggingConfiguration
	offline           bool
}

func (c *basicClientContext) GetSDKKey() string {
	return c.sdkKey
}

func (c *basicClientContext) GetDefaultHTTPHeaders() http.Header {
	return c.headers
}

func (c *basicClientContext) CreateHTTPClient() *http.Client {
	if c.httpClientFactory == nil {
		return http.DefaultClient
	}
	return c.httpClientFactory()
}

func (c *basicClientContext) GetLogging() LoggingConfiguration {
	return c.logging
}

func (c *basicClientContext) IsOffline() bool {
	return c.offline
}

// NewClientContext creates the default implementation of ClientContext with the provided values.
//
// If httpClientFactory is nil, components will use http.DefaultClient.
//
// To turn off logging for test code, set logging to ldcomponents.NoLogging().CreateLoggingConfiguration().
func NewClientContext(sdkKey string, headers http.Header, httpClientFactory func() *http.Client, logging LoggingConfiguration) ClientContext {
	if logging == nil {
		logging = defaultLoggingConfiguration{}
	}
	return &basicClientContext{sdkKey, headers, httpClientFactory, logging, false}
}

type defaultLoggingConfiguration struct{}

func (c defaultLoggingConfiguration) IsLogEvaluationErrors() bool {
	return false
}

func (c defaultLoggingConfiguration) IsLogUserKeyInErrors() bool {
	return false
}

func (c defaultLoggingConfiguration) GetLoggers() ldlog.Loggers {
	return ldlog.NewDefaultLoggers()
}

func (c defaultLoggingConfiguration) GetLogDataSourceOutageAsErrorAfter() time.Duration {
	return 0
}

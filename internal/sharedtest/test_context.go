package sharedtest

import (
	"net/http"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
)

type stubClientContext struct {
	sdkKey  string
	http    interfaces.HTTPConfiguration
	logging interfaces.LoggingConfiguration
}

// NewSimpleTestContext returns a basic implementation of interfaces.ClientContext for use in test code.
func NewSimpleTestContext(sdkKey string) interfaces.ClientContext {
	return NewTestContext(sdkKey, TestHTTPConfig(), TestLoggingConfig())
}

// NewTestContext returns a basic implementation of interfaces.ClientContext for use in test code.
// We can't use internal.NewClientContextImpl for this because of circular references.
func NewTestContext(
	sdkKey string,
	http interfaces.HTTPConfiguration,
	logging interfaces.LoggingConfiguration,
) interfaces.ClientContext {
	return stubClientContext{sdkKey, http, logging}
}

func (c stubClientContext) GetBasic() interfaces.BasicConfiguration {
	return interfaces.BasicConfiguration{SDKKey: c.sdkKey}
}

func (c stubClientContext) GetHTTP() interfaces.HTTPConfiguration {
	return c.http
}

func (c stubClientContext) GetLogging() interfaces.LoggingConfiguration {
	return c.logging
}

// TestHTTP returns a basic HTTPConfigurationFactory for test code.
func TestHTTP() interfaces.HTTPConfigurationFactory {
	return testHTTPConfigurationFactory{}
}

// TestHTTPConfig returns a basic HTTPConfiguration for test code.
func TestHTTPConfig() interfaces.HTTPConfiguration {
	return testHTTPConfiguration{}
}

// TestHTTPConfigWithHeaders returns a basic HTTPConfiguration with the specified HTTP headers.
func TestHTTPConfigWithHeaders(headers http.Header) interfaces.HTTPConfiguration {
	return testHTTPConfiguration{headers}
}

// TestLogging returns a LoggingConfigurationFactory corresponding to NewTestLoggers().
func TestLogging() interfaces.LoggingConfigurationFactory {
	return testLoggingConfigurationFactory{}
}

// TestLoggingConfig returns a LoggingConfiguration corresponding to NewTestLoggers().
func TestLoggingConfig() interfaces.LoggingConfiguration {
	return testLoggingConfiguration{NewTestLoggers()}
}

// TestLoggingConfigWithLoggers returns a LoggingConfiguration with the specified Loggers.
func TestLoggingConfigWithLoggers(loggers ldlog.Loggers) interfaces.LoggingConfiguration {
	return testLoggingConfiguration{loggers}
}

type testHTTPConfiguration struct{ headers http.Header }

type testHTTPConfigurationFactory struct{}

func (c testHTTPConfiguration) GetDefaultHeaders() http.Header {
	return c.headers
}

func (c testHTTPConfiguration) CreateHTTPClient() *http.Client {
	client := *http.DefaultClient
	return &client
}

func (c testHTTPConfigurationFactory) CreateHTTPConfiguration(
	basicConfig interfaces.BasicConfiguration,
) (interfaces.HTTPConfiguration, error) {
	return testHTTPConfiguration{}, nil
}

type testLoggingConfiguration struct {
	loggers ldlog.Loggers
}

type testLoggingConfigurationFactory struct{}

func (c testLoggingConfiguration) IsLogEvaluationErrors() bool {
	return false
}

func (c testLoggingConfiguration) IsLogUserKeyInErrors() bool {
	return false
}

func (c testLoggingConfiguration) GetLogDataSourceOutageAsErrorAfter() time.Duration {
	return 0
}

func (c testLoggingConfiguration) GetLoggers() ldlog.Loggers {
	return c.loggers
}

func (c testLoggingConfigurationFactory) CreateLoggingConfiguration(
	basicConfig interfaces.BasicConfiguration,
) (interfaces.LoggingConfiguration, error) {
	return testLoggingConfiguration{NewTestLoggers()}, nil
}

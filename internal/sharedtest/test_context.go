package sharedtest

import (
	"net/http"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
)

type stubClientContext struct {
	sdkKey  string
	http    interfaces.HTTPConfiguration
	logging interfaces.LoggingConfiguration
}

// NewSimpleTestContext returns a basic implementation of interfaces.ClientContext for use in test code.
func NewSimpleTestContext(sdkKey string) interfaces.ClientContext {
	return NewTestContext(sdkKey, nil, nil)
}

// NewTestContext returns a basic implementation of interfaces.ClientContext for use in test code.
// We can't use internal.NewClientContextImpl for this because of circular references.
func NewTestContext(
	sdkKey string,
	optHTTPConfig *interfaces.HTTPConfiguration,
	optLoggingConfig *interfaces.LoggingConfiguration,
) interfaces.ClientContext {
	var httpConfig interfaces.HTTPConfiguration
	if optHTTPConfig != nil {
		httpConfig = *optHTTPConfig
	}
	if httpConfig.CreateHTTPClient == nil {
		httpConfig.CreateHTTPClient = func() *http.Client {
			client := *http.DefaultClient
			return &client
		}
	}
	var loggingConfig interfaces.LoggingConfiguration
	if optLoggingConfig != nil {
		loggingConfig = *optLoggingConfig
	} else {
		loggingConfig.Loggers = NewTestLoggers()
	}
	return stubClientContext{sdkKey, httpConfig, loggingConfig}
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

// TestLoggingConfig returns a LoggingConfiguration corresponding to NewTestLoggers().
func TestLoggingConfig() interfaces.LoggingConfiguration {
	return interfaces.LoggingConfiguration{Loggers: NewTestLoggers()}
}

// TestLogging returns a LoggingConfigurationFactory corresponding to NewTestLoggers().
func TestLogging() interfaces.LoggingConfigurationFactory {
	return testLoggingConfigurationFactory{}
}

type testLoggingConfigurationFactory struct{}

func (c testLoggingConfigurationFactory) CreateLoggingConfiguration(
	basicConfig interfaces.BasicConfiguration,
) (interfaces.LoggingConfiguration, error) {
	return TestLoggingConfig(), nil
}

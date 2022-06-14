package sharedtest

import (
	"net/http"

	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

type stubClientContext struct {
	sdkKey  string
	http    subsystems.HTTPConfiguration
	logging subsystems.LoggingConfiguration
}

// NewSimpleTestContext returns a basic implementation of interfaces.ClientContext for use in test code.
func NewSimpleTestContext(sdkKey string) subsystems.ClientContext {
	return NewTestContext(sdkKey, nil, nil)
}

// NewTestContext returns a basic implementation of interfaces.ClientContext for use in test code.
// We can't use internal.NewClientContextImpl for this because of circular references.
func NewTestContext(
	sdkKey string,
	optHTTPConfig *subsystems.HTTPConfiguration,
	optLoggingConfig *subsystems.LoggingConfiguration,
) subsystems.ClientContext {
	var httpConfig subsystems.HTTPConfiguration
	if optHTTPConfig != nil {
		httpConfig = *optHTTPConfig
	}
	if httpConfig.CreateHTTPClient == nil {
		httpConfig.CreateHTTPClient = func() *http.Client {
			client := *http.DefaultClient
			return &client
		}
	}
	var loggingConfig subsystems.LoggingConfiguration
	if optLoggingConfig != nil {
		loggingConfig = *optLoggingConfig
	} else {
		loggingConfig.Loggers = NewTestLoggers()
	}
	return stubClientContext{sdkKey, httpConfig, loggingConfig}
}

func (c stubClientContext) GetBasic() subsystems.BasicConfiguration {
	return subsystems.BasicConfiguration{SDKKey: c.sdkKey}
}

func (c stubClientContext) GetHTTP() subsystems.HTTPConfiguration {
	return c.http
}

func (c stubClientContext) GetLogging() subsystems.LoggingConfiguration {
	return c.logging
}

// TestLoggingConfig returns a LoggingConfiguration corresponding to NewTestLoggers().
func TestLoggingConfig() subsystems.LoggingConfiguration {
	return subsystems.LoggingConfiguration{Loggers: NewTestLoggers()}
}

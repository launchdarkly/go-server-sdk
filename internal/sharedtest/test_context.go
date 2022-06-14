package sharedtest

import (
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

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
) subsystems.BasicClientContext {
	ret := subsystems.BasicClientContext{SDKKey: sdkKey}
	if optHTTPConfig != nil {
		ret.HTTP = *optHTTPConfig
	}
	if optLoggingConfig != nil {
		ret.Logging = *optLoggingConfig
	} else {
		ret.Logging = TestLoggingConfig()
	}
	return ret
}

// TestLoggingConfig returns a LoggingConfiguration corresponding to NewTestLoggers().
func TestLoggingConfig() subsystems.LoggingConfiguration {
	return subsystems.LoggingConfiguration{Loggers: NewTestLoggers()}
}

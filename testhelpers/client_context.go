package testhelpers

import (
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

// SimpleClientContext is a reference implementation of interfaces.ClientContext for test code.
//
// The SDK uses the ClientContext interface to pass its configuration to subcomponents. Its standard
// implementation also contains other environment information that is only relevant to built-in SDK
// code. SimpleClientContext may be useful for external code to test a custom component.
type SimpleClientContext struct {
	sdkKey  string
	http    interfaces.HTTPConfiguration
	logging interfaces.LoggingConfiguration
}

// NewSimpleClientContext creates a SimpleClientContext instance, with a standard HTTP configuration
// and a disabled logging configuration.
func NewSimpleClientContext(sdkKey string) SimpleClientContext {
	return SimpleClientContext{sdkKey: sdkKey}
}

func (s SimpleClientContext) GetBasic() interfaces.BasicConfiguration { //nolint:golint
	return interfaces.BasicConfiguration{SDKKey: s.sdkKey, Offline: false}
}

func (s SimpleClientContext) GetHTTP() interfaces.HTTPConfiguration { //nolint:golint
	if s.http != nil {
		return s.http
	}
	c, _ := ldcomponents.HTTPConfiguration().CreateHTTPConfiguration(s.GetBasic())
	return c
}

func (s SimpleClientContext) GetLogging() interfaces.LoggingConfiguration { //nolint:golint
	if s.logging != nil {
		return s.logging
	}
	c, _ := ldcomponents.Logging().CreateLoggingConfiguration(s.GetBasic())
	return c
}

// WithHTTP returns a new SimpleClientContext based on the original one, but adding the specified
// HTTP configuration.
func (s SimpleClientContext) WithHTTP(httpConfig interfaces.HTTPConfigurationFactory) SimpleClientContext {
	ret := s
	ret.http, _ = httpConfig.CreateHTTPConfiguration(s.GetBasic())
	return ret
}

// WithLogging returns a new SimpleClientContext based on the original one, but adding the specified
// logging configuration.
func (s SimpleClientContext) WithLogging(loggingConfig interfaces.LoggingConfigurationFactory) SimpleClientContext {
	ret := s
	ret.logging, _ = loggingConfig.CreateLoggingConfiguration(s.GetBasic())
	return ret
}

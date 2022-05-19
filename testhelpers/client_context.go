package testhelpers

import (
	"net/http"
	"os"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// SimpleClientContext is a reference implementation of interfaces.ClientContext for test code.
//
// The SDK uses the ClientContext interface to pass its configuration to subcomponents. Its standard
// implementation also contains other environment information that is only relevant to built-in SDK
// code. SimpleClientContext may be useful for external code to test a custom component.
type SimpleClientContext struct {
	sdkKey  string
	http    *subsystems.HTTPConfiguration
	logging *subsystems.LoggingConfiguration
}

// NewSimpleClientContext creates a SimpleClientContext instance, with a standard HTTP configuration
// and a disabled logging configuration.
func NewSimpleClientContext(sdkKey string) SimpleClientContext {
	return SimpleClientContext{sdkKey: sdkKey}
}

func (s SimpleClientContext) GetBasic() subsystems.BasicConfiguration { //nolint:revive
	return subsystems.BasicConfiguration{SDKKey: s.sdkKey, Offline: false}
}

func (s SimpleClientContext) GetHTTP() subsystems.HTTPConfiguration { //nolint:revive
	if s.http != nil {
		ret := *s.http
		if ret.CreateHTTPClient == nil {
			ret.CreateHTTPClient = func() *http.Client {
				client := *http.DefaultClient
				return &client
			}
		}
		return *s.http
	}
	c, _ := ldcomponents.HTTPConfiguration().CreateHTTPConfiguration(s.GetBasic())
	return c
}

func (s SimpleClientContext) GetLogging() subsystems.LoggingConfiguration { //nolint:revive
	if s.logging != nil {
		return *s.logging
	}
	return ldcomponents.Logging().CreateLoggingConfiguration(s.GetBasic())
}

// WithHTTP returns a new SimpleClientContext based on the original one, but adding the specified
// HTTP configuration.
func (s SimpleClientContext) WithHTTP(httpConfig *ldcomponents.HTTPConfigurationBuilder) SimpleClientContext {
	if httpConfig == nil {
		httpConfig = ldcomponents.HTTPConfiguration()
	}
	config, _ := httpConfig.CreateHTTPConfiguration(s.GetBasic())
	ret := s
	ret.http = &config
	return ret
}

// WithLogging returns a new SimpleClientContext based on the original one, but adding the specified
// logging configuration.
func (s SimpleClientContext) WithLogging(loggingConfig *ldcomponents.LoggingConfigurationBuilder) SimpleClientContext {
	if loggingConfig == nil {
		loggingConfig = ldcomponents.Logging()
	}
	config := loggingConfig.CreateLoggingConfiguration(s.GetBasic())
	ret := s
	ret.logging = &config
	return ret
}

// Fallible is a general interface for anything with a Failed method. This is used by test helpers to
// generalize between *testing.T, assert.T, etc. when all that we care about is detecting test failure.
type Fallible interface {
	Failed() bool
}

// WithMockLoggingContext creates a ClientContext that writes to a MockLogger, executes the specified
// action, and then dumps the captured output to the console only if there's been a test failure.
func WithMockLoggingContext(t Fallible, action func(subsystems.ClientContext)) {
	mockLog := ldlogtest.NewMockLog()
	context := sharedtest.NewTestContext("", &subsystems.HTTPConfiguration{},
		&subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
	defer func() {
		if t.Failed() {
			mockLog.Dump(os.Stdout)
		}
		// There's already a similar DumpLogIfTestFailed defined in the ldlogtest package, but it requires
		// specifically a *testing.T.
	}()
	action(context)
}

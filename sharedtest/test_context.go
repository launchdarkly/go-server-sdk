package sharedtest

import (
	"net/http"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type stubClientContext struct{}

// TestLogging returns a LoggingConfiguration corresponding to NewTestLoggers().
func TestLogging() interfaces.LoggingConfiguration {
	return testLoggingConfiguration{}
}

func (c stubClientContext) GetSDKKey() string {
	return "test-sdk-key"
}

func (c stubClientContext) GetDefaultHTTPHeaders() http.Header {
	return nil
}

func (c stubClientContext) CreateHTTPClient() *http.Client {
	return http.DefaultClient
}

func (c stubClientContext) GetLogging() interfaces.LoggingConfiguration {
	return TestLogging()
}

type testLoggingConfiguration struct{}

func (c testLoggingConfiguration) IsLogEvaluationErrors() bool {
	return false
}

func (c testLoggingConfiguration) IsLogUserKeyInErrors() bool {
	return false
}

func (c testLoggingConfiguration) GetLoggers() ldlog.Loggers {
	return NewTestLoggers()
}

func (c stubClientContext) IsOffline() bool {
	return false
}

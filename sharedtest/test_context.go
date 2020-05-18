package sharedtest

import (
	"net/http"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

type stubClientContext struct{}

func (c stubClientContext) GetSDKKey() string {
	return "test-sdk-key"
}

func (c stubClientContext) GetDefaultHTTPHeaders() http.Header {
	return nil
}

func (c stubClientContext) CreateHTTPClient() *http.Client {
	return http.DefaultClient
}

func (c stubClientContext) GetLoggers() ldlog.Loggers {
	return NewTestLoggers()
}

func (c stubClientContext) IsOffline() bool {
	return false
}

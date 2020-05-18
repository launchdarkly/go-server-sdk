package ldcomponents

import (
	"net/http"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

const testSdkKey = "test-sdk-key"

func basicClientContext() interfaces.ClientContext {
	return interfaces.NewClientContext(testSdkKey, nil, nil, sharedtest.TestLogging())
}

type contextWithDiagnostics struct {
	sdkKey             string
	headers            http.Header
	httpClientFactory  func() *http.Client
	diagnosticsManager *ldevents.DiagnosticsManager
}

func (c *contextWithDiagnostics) GetSDKKey() string {
	return c.sdkKey
}

func (c *contextWithDiagnostics) GetLogging() interfaces.LoggingConfiguration {
	return sharedtest.TestLogging()
}

func (c *contextWithDiagnostics) GetDefaultHTTPHeaders() http.Header {
	return c.headers
}

func (c *contextWithDiagnostics) CreateHTTPClient() *http.Client {
	if c.httpClientFactory == nil {
		return http.DefaultClient
	}
	return c.httpClientFactory()
}

func (c *contextWithDiagnostics) GetDiagnosticsManager() *ldevents.DiagnosticsManager {
	return c.diagnosticsManager
}

func (c *contextWithDiagnostics) IsOffline() bool {
	return false
}

func newClientContextWithDiagnostics(sdkKey string, headers http.Header, httpClientFactory func() *http.Client, diagnosticsManager *ldevents.DiagnosticsManager) interfaces.ClientContext {
	return &contextWithDiagnostics{sdkKey, headers, httpClientFactory, diagnosticsManager}
}

func makeInMemoryDataStore() interfaces.DataStore {
	return internal.NewInMemoryDataStore(sharedtest.NewTestLoggers())
}

type urlAppendingHTTPTransport string

func urlAppendingHTTPClientFactory(suffix string) func() *http.Client {
	return func() *http.Client {
		return &http.Client{Transport: urlAppendingHTTPTransport(suffix)}
	}
}

func (t urlAppendingHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	req := *r
	req.URL.Path = req.URL.Path + string(t)
	return http.DefaultTransport.RoundTrip(&req)
}

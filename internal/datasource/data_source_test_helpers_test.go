package datasource

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

const testSDKKey = "test-sdk-key"

func basicClientContext() interfaces.ClientContext {
	return sharedtest.NewSimpleTestContext(testSDKKey)
}

func withMockDataSourceUpdates(action func(*sharedtest.MockDataSourceUpdates)) {
	d := sharedtest.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	// currently don't need to defer any cleanup actions
	action(d)
}

func waitForReadyWithTimeout(t *testing.T, closeWhenReady <-chan struct{}, timeout time.Duration) {
	select {
	case <-closeWhenReady:
		return
	case <-time.After(timeout):
		require.Fail(t, "timed out waiting for data source to finish starting")
	}
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

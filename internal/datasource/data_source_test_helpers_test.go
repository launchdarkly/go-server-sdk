package datasource

import (
	"net/http"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	th "github.com/launchdarkly/go-test-helpers/v3"
)

const testSDKKey = "test-sdk-key"

func basicClientContext() subsystems.ClientContext {
	return sharedtest.NewSimpleTestContext(testSDKKey)
}

func withMockDataSourceUpdates(action func(*mocks.MockDataSourceUpdates)) {
	d := mocks.NewMockDataSourceUpdates(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	// currently don't need to defer any cleanup actions
	action(d)
}

func waitForReadyWithTimeout(t *testing.T, closeWhenReady <-chan struct{}, timeout time.Duration) {
	if !th.AssertChannelClosed(t, closeWhenReady, timeout) {
		t.FailNow()
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

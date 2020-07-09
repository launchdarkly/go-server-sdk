// +build proxytest1

// Note, the tests in this package must be run one at a time in separate "go test" invocations, because
// (depending on the platform) Go may cache the value of HTTP_PROXY. Therefore, we have a separate build
// tag for each test and the Makefile runs this package once for each tag.

package proxytest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"gopkg.in/launchdarkly/go-server-sdk.v5/ldhttp"
)

func TestDefaultTransportUsesProxyEnvVars(t *testing.T) {
	oldHttpProxy := os.Getenv("HTTP_PROXY")
	defer os.Setenv("HTTP_PROXY", oldHttpProxy)

	targetURL := "http://badhost/url"

	// Create an extremely minimal fake proxy server that doesn't actually do any proxying, just to
	// verify that we are connecting to it. If the HTTP_PROXY setting is ignored, then it will try
	// to connect directly to the nonexistent host "badhost" instead and get an error.
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(200))
	httphelpers.WithServer(handler, func(proxy *httptest.Server) {
		// Note that in normal usage, we will be connecting to secure LaunchDarkly endpoints, so it's
		// really HTTPS_PROXY that is relevant. But support for HTTP_PROXY and HTTPS_PROXY comes from the
		// same mechanism, so it's simpler to just test against an insecure proxy.
		os.Setenv("HTTP_PROXY", proxy.URL)

		transport, _, err := ldhttp.NewHTTPTransport()
		require.NoError(t, err)

		client := *http.DefaultClient
		client.Transport = transport
		resp, err := client.Get(targetURL)
		require.NoError(t, err)

		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 1, len(requestsCh))
		r := <-requestsCh
		assert.Equal(t, targetURL, r.Request.URL.String())
	})
}

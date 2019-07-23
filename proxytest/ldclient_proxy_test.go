// +build proxytest2

// Note, the tests in this package must be run one at a time in separate "go test" invocations, because
// (depending on the platform) Go may cache the value of HTTP_PROXY. Therefore, we have a separate build
// tag for each test and the Makefile runs this package once for each tag.

package proxytest

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
	shared "gopkg.in/launchdarkly/go-server-sdk.v4/shared_test"
)

func TestClientUsesProxyEnvVars(t *testing.T) {
	oldHttpProxy := os.Getenv("HTTP_PROXY")
	defer os.Setenv("HTTP_PROXY", oldHttpProxy)

	fakeBaseURL := "http://badhost/url"
	fakeEndpointURL := fakeBaseURL + "/sdk/latest-all"

	// Create an extremely minimal fake proxy server that doesn't actually do any proxying, just to
	// verify that we are connecting to it. If the HTTP_PROXY setting is ignored, then it will try
	// to connect directly to the nonexistent host "badhost" instead and get an error.
	proxy := shared.NewStubHTTPServer(shared.StubResponse{Code: 200, Body: "{}"})
	defer proxy.Close()

	// Note that in normal usage, we will be connecting to secure LaunchDarkly endpoints, so it's
	// really HTTPS_PROXY that is relevant. But support for HTTP_PROXY and HTTPS_PROXY comes from the
	// same mechanism, so it's simpler to just test against an insecure proxy.
	os.Setenv("HTTP_PROXY", proxy.URL)

	config := ld.DefaultConfig
	config.Logger = log.New(ioutil.Discard, "", 0)
	config.BaseUri = fakeBaseURL
	config.SendEvents = false
	config.Stream = false
	
	client, err := ld.MakeCustomClient("sdkKey", config, 5 * time.Second)
	require.NoError(t, err)
	defer client.Close()

	assert.Equal(t, []string{fakeEndpointURL}, proxy.RequestedURLs)
}

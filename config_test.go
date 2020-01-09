package ldclient

import (
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v4/ldhttp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type urlAppendingHTTPTransport string

func urlAppendingHTTPClientFactory(suffix string) func(Config) http.Client {
	return func(Config) http.Client {
		return http.Client{Transport: urlAppendingHTTPTransport(suffix)}
	}
}

func (t urlAppendingHTTPTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	req := *r
	req.URL.Path = req.URL.Path + string(t)
	return http.DefaultTransport.RoundTrip(&req)
}

func TestNewHTTPClientFactorySetsDefaults(t *testing.T) {
	cf := NewHTTPClientFactory()
	config := DefaultConfig
	config.Timeout = 45 * time.Second
	client := cf(config)
	assert.Equal(t, config.Timeout, client.Timeout)

	require.NotNil(t, client.Transport)
	transport := client.Transport.(*http.Transport)
	require.NotNil(t, transport)
	assert.Equal(t, reflect.ValueOf(http.ProxyFromEnvironment).Pointer(), reflect.ValueOf(transport.Proxy).Pointer())
	assert.Equal(t, 100, transport.MaxIdleConns)
	assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
	assert.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
	assert.Equal(t, 1*time.Second, transport.ExpectContinueTimeout)
}

func TestNewHTTPClientFactoryCanSetProxyURL(t *testing.T) {
	url, err := url.Parse("https://fake-proxy")
	require.NoError(t, err)
	cf := NewHTTPClientFactory(ldhttp.ProxyOption(*url))
	client := cf(DefaultConfig)

	require.NotNil(t, client.Transport)
	transport := client.Transport.(*http.Transport)
	require.NotNil(t, transport)
	require.NotNil(t, transport.Proxy)
	urlOut, err := transport.Proxy(&http.Request{})
	require.NoError(t, err)
	assert.Equal(t, url, urlOut)
}

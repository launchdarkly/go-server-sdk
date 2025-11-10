package ldhttp

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helpers "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
)

// See also: proxytest/http_transport_proxy_test.go

func TestDefaultTransportDoesNotAcceptSelfSignedCert(t *testing.T) {
	alwaysOK := httphelpers.HandlerWithStatus(200)
	httphelpers.WithSelfSignedServer(alwaysOK, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
		transport, _, err := NewHTTPTransport()
		require.NoError(t, err)

		client := *http.DefaultClient
		client.Transport = transport
		_, err = client.Get(server.URL)
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "certificate") // the exact error message varies by Go version
	})
}

func TestCanAcceptSelfSignedCertWithCA(t *testing.T) {
	alwaysOK := httphelpers.HandlerWithStatus(200)
	httphelpers.WithSelfSignedServer(alwaysOK, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
		transport, _, err := NewHTTPTransport(CACertOption(certData))
		require.NoError(t, err)

		client := *http.DefaultClient
		client.Transport = transport
		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
}

func TestErrorForNonexistentCertFile(t *testing.T) {
	helpers.WithTempFile(func(certFile string) {
		os.Remove(certFile)
		_, _, err := NewHTTPTransport(CACertFileOption(certFile))
		require.Error(t, err)
		require.Contains(t, err.Error(), "can't read CA certificate file")
	})
}

func TestErrorForCertFileWithBadData(t *testing.T) {
	helpers.WithTempFile(func(certFile string) {
		os.WriteFile(certFile, []byte("sorry"), os.ModeAppend)
		_, _, err := NewHTTPTransport(CACertFileOption(certFile))
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid CA certificate data")
	})
}

func TestErrorForBadCertData(t *testing.T) {
	_, _, err := NewHTTPTransport(CACertOption([]byte("sorry")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid CA certificate data")
}

func TestProxyEnvVarsAreUsedByDefault(t *testing.T) {
	transport, _, err := NewHTTPTransport()
	require.NoError(t, err)
	require.NotNil(t, transport.Proxy)
	assert.Equal(t, reflect.ValueOf(http.ProxyFromEnvironment).Pointer(), reflect.ValueOf(transport.Proxy).Pointer())
}

func TestCanSetProxyURL(t *testing.T) {
	url, err := url.Parse("https://fake-proxy")
	require.NoError(t, err)
	transport, _, err := NewHTTPTransport(ProxyOption(*url))
	require.NoError(t, err)
	require.NotNil(t, transport.Proxy)
	urlOut, err := transport.Proxy(&http.Request{})
	require.NoError(t, err)
	assert.Equal(t, url, urlOut)
}

func TestCanSetIdleConnTimeout(t *testing.T) {
	customTimeout := 30 * time.Second
	transport, _, err := NewHTTPTransport(IdleConnTimeoutOption(customTimeout))
	require.NoError(t, err)
	assert.Equal(t, customTimeout, transport.IdleConnTimeout)
}

func TestDefaultIdleConnTimeoutIsUsedWhenNotSet(t *testing.T) {
	transport, _, err := NewHTTPTransport()
	require.NoError(t, err)
	// Should use the default from newDefaultTransport
	assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
}

func TestCanSetMaxIdleConns(t *testing.T) {
	customMax := 50
	transport, _, err := NewHTTPTransport(MaxIdleConnsOption(customMax))
	require.NoError(t, err)
	assert.Equal(t, customMax, transport.MaxIdleConns)
}

func TestDefaultMaxIdleConnsIsUsedWhenNotSet(t *testing.T) {
	transport, _, err := NewHTTPTransport()
	require.NoError(t, err)
	// Should use the default from newDefaultTransport
	assert.Equal(t, 100, transport.MaxIdleConns)
}

func TestCanSetMaxIdleConnsPerHost(t *testing.T) {
	customMax := 10
	transport, _, err := NewHTTPTransport(MaxIdleConnsPerHostOption(customMax))
	require.NoError(t, err)
	assert.Equal(t, customMax, transport.MaxIdleConnsPerHost)
}

func TestDefaultMaxIdleConnsPerHostIsUsedWhenNotSet(t *testing.T) {
	transport, _, err := NewHTTPTransport()
	require.NoError(t, err)
	// Should be 0 (no limit) when not set
	assert.Equal(t, 0, transport.MaxIdleConnsPerHost)
}

func TestCanDisableKeepAlives(t *testing.T) {
	transport, _, err := NewHTTPTransport(DisableKeepAlivesOption(true))
	require.NoError(t, err)
	assert.True(t, transport.DisableKeepAlives)
}

func TestKeepAlivesEnabledByDefault(t *testing.T) {
	transport, _, err := NewHTTPTransport()
	require.NoError(t, err)
	assert.False(t, transport.DisableKeepAlives)
}

func TestCanSetMultipleConnectionOptions(t *testing.T) {
	customIdleTimeout := 45 * time.Second
	customMaxIdle := 75
	customMaxIdlePerHost := 15

	transport, _, err := NewHTTPTransport(
		IdleConnTimeoutOption(customIdleTimeout),
		MaxIdleConnsOption(customMaxIdle),
		MaxIdleConnsPerHostOption(customMaxIdlePerHost),
		DisableKeepAlivesOption(true),
	)
	require.NoError(t, err)
	assert.Equal(t, customIdleTimeout, transport.IdleConnTimeout)
	assert.Equal(t, customMaxIdle, transport.MaxIdleConns)
	assert.Equal(t, customMaxIdlePerHost, transport.MaxIdleConnsPerHost)
	assert.True(t, transport.DisableKeepAlives)
}

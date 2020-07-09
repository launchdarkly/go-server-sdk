package ldcomponents

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	helpers "github.com/launchdarkly/go-test-helpers/v2"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/assert"
)

func TestHTTPConfigurationBuilder(t *testing.T) {
	basicConfig := interfaces.BasicConfiguration{SDKKey: "test-key"}

	t.Run("defaults", func(t *testing.T) {
		c, err := HTTPConfiguration().CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		headers := c.GetDefaultHeaders()
		assert.Len(t, headers, 2)
		assert.Equal(t, "test-key", headers.Get("Authorization"))
		assert.Equal(t, "GoClient/"+internal.SDKVersion, headers.Get("User-Agent"))

		client := c.CreateHTTPClient()
		assert.Equal(t, DefaultConnectTimeout, client.Timeout)

		require.NotNil(t, client.Transport)
		transport := client.Transport.(*http.Transport)
		require.NotNil(t, transport)
		assert.Equal(t, reflect.ValueOf(http.ProxyFromEnvironment).Pointer(), reflect.ValueOf(transport.Proxy).Pointer())
		assert.Equal(t, 100, transport.MaxIdleConns)
		assert.Equal(t, 90*time.Second, transport.IdleConnTimeout)
		assert.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
		assert.Equal(t, 1*time.Second, transport.ExpectContinueTimeout)
	})

	t.Run("CACert", func(t *testing.T) {
		httphelpers.WithSelfSignedServer(httphelpers.HandlerWithStatus(200), func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
			_, err := HTTPConfiguration().
				CACert(certData).
				CreateHTTPConfiguration(basicConfig)
			require.NoError(t, err)
		})
	})

	t.Run("CACert with invalid certificate", func(t *testing.T) {
		badCertData := []byte("no")
		_, err := HTTPConfiguration().
			CACert(badCertData).
			CreateHTTPConfiguration(basicConfig)
		require.Error(t, err)
	})

	t.Run("CACertFile", func(t *testing.T) {
		httphelpers.WithSelfSignedServer(httphelpers.HandlerWithStatus(200), func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
			sharedtest.WithTempFileContaining(certData, func(filename string) {
				_, err := HTTPConfiguration().
					CACertFile(filename).
					CreateHTTPConfiguration(basicConfig)
				require.NoError(t, err)
			})
		})
	})

	t.Run("CACertFile with invalid certificate", func(t *testing.T) {
		badCertData := []byte("no")
		sharedtest.WithTempFileContaining(badCertData, func(filename string) {
			_, err := HTTPConfiguration().
				CACertFile(filename).
				CreateHTTPConfiguration(basicConfig)
			require.Error(t, err)
		})
	})

	t.Run("CACertFile with missing file", func(t *testing.T) {
		helpers.WithTempFile(func(filename string) {
			_ = os.Remove(filename)
			_, err := HTTPConfiguration().
				CACertFile(filename).
				CreateHTTPConfiguration(basicConfig)
			require.Error(t, err)
		})
	})

	t.Run("ConnectTimeout", func(t *testing.T) {
		timeout := 700 * time.Millisecond
		c1, err := HTTPConfiguration().
			ConnectTimeout(timeout).
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		client1 := c1.CreateHTTPClient()
		assert.Equal(t, timeout, client1.Timeout)

		c2, err := HTTPConfiguration().
			ConnectTimeout(-1 * time.Millisecond).
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		client2 := c2.CreateHTTPClient()
		assert.Equal(t, DefaultConnectTimeout, client2.Timeout)

	})

	t.Run("HTTPClientFactory", func(t *testing.T) {
		hc := &http.Client{Timeout: time.Hour}

		c, err := HTTPConfiguration().
			HTTPClientFactory(func() *http.Client { return hc }).
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		assert.Equal(t, hc, c.CreateHTTPClient())
	})

	t.Run("ProxyURL", func(t *testing.T) {
		proxyURL := "https://fake-proxy"
		parsedURL, err := url.Parse(proxyURL)
		require.NoError(t, err)

		c, err := HTTPConfiguration().
			ProxyURL(proxyURL).
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		client := c.CreateHTTPClient()

		require.NotNil(t, client.Transport)
		transport := client.Transport.(*http.Transport)
		require.NotNil(t, transport)
		require.NotNil(t, transport.Proxy)
		urlOut, err := transport.Proxy(&http.Request{})
		require.NoError(t, err)
		assert.Equal(t, parsedURL, urlOut)
	})

	t.Run("ProxyURL with invalid URL", func(t *testing.T) {
		proxyURL := ":///"

		_, err := HTTPConfiguration().
			ProxyURL(proxyURL).
			CreateHTTPConfiguration(basicConfig)
		require.Error(t, err)
	})

	t.Run("User-Agent", func(t *testing.T) {
		c, err := HTTPConfiguration().
			UserAgent("extra").
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		headers := c.GetDefaultHeaders()
		assert.Equal(t, "GoClient/"+internal.SDKVersion+" extra", headers.Get("User-Agent"))
	})

	t.Run("Wrapper", func(t *testing.T) {
		c1, err := HTTPConfiguration().
			Wrapper("FancySDK", "").
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		headers1 := c1.GetDefaultHeaders()
		assert.Equal(t, "FancySDK", headers1.Get("X-LaunchDarkly-Wrapper"))

		c2, err := HTTPConfiguration().
			Wrapper("FancySDK", "2.0").
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		headers2 := c2.GetDefaultHeaders()
		assert.Equal(t, "FancySDK/2.0", headers2.Get("X-LaunchDarkly-Wrapper"))
	})
}

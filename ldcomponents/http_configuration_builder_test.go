package ldcomponents

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/httphelpers"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

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

	t.Run("can set CA certs", func(t *testing.T) {
		httphelpers.WithSelfSignedServer(httphelpers.HandlerWithStatus(200), func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
			_, err := HTTPConfiguration().
				CACert(certData).
				CreateHTTPConfiguration(basicConfig)
			require.NoError(t, err)

			sharedtest.WithTempFileContaining(certData, func(filename string) {
				_, err := HTTPConfiguration().
					CACertFile(filename).
					CreateHTTPConfiguration(basicConfig)
				require.NoError(t, err)
			})
		})
	})

	t.Run("bad CA certs are rejected", func(t *testing.T) {
		badCertData := []byte("no")

		_, err := HTTPConfiguration().
			CACert(badCertData).
			CreateHTTPConfiguration(basicConfig)
		require.Error(t, err)

		sharedtest.WithTempFileContaining(badCertData, func(filename string) {
			_, err := HTTPConfiguration().
				CACertFile(filename).
				CreateHTTPConfiguration(basicConfig)
			require.Error(t, err)
		})
	})

	t.Run("can set connect timeout", func(t *testing.T) {
		timeout := 700 * time.Millisecond
		c, err := HTTPConfiguration().
			ConnectTimeout(timeout).
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		client := c.CreateHTTPClient()
		assert.Equal(t, timeout, client.Timeout)
	})

	t.Run("can set proxy URL", func(t *testing.T) {
		url, err := url.Parse("https://fake-proxy")
		require.NoError(t, err)

		c, err := HTTPConfiguration().
			ProxyURL(*url).
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		client := c.CreateHTTPClient()

		require.NotNil(t, client.Transport)
		transport := client.Transport.(*http.Transport)
		require.NotNil(t, transport)
		require.NotNil(t, transport.Proxy)
		urlOut, err := transport.Proxy(&http.Request{})
		require.NoError(t, err)
		assert.Equal(t, url, urlOut)
	})

	t.Run("can set User-Agent", func(t *testing.T) {
		c, err := HTTPConfiguration().
			UserAgent("extra").
			CreateHTTPConfiguration(basicConfig)
		require.NoError(t, err)

		headers := c.GetDefaultHeaders()
		assert.Equal(t, "GoClient/"+internal.SDKVersion+" extra", headers.Get("User-Agent"))
	})

	t.Run("can set wrapper identifier", func(t *testing.T) {
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

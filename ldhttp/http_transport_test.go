package ldhttp

import (
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	shared "gopkg.in/launchdarkly/go-server-sdk.v4/shared_test"
)

func TestDefaultTransportDoesNotAcceptSelfSignedCert(t *testing.T) {
	shared.WithTempFile(func(certFile string) {
		shared.WithTempFile(func(keyFile string) {
			err := shared.MakeSelfSignedCert(certFile, keyFile)
			require.NoError(t, err)

			server, err := shared.MakeServerWithCert(certFile, keyFile, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(200)
			}))
			require.NoError(t, err)
			defer server.Close()

			transport, _, err := NewHTTPTransport()
			require.NoError(t, err)

			client := *http.DefaultClient
			client.Transport = transport
			_, err = client.Get(server.URL)
			require.NotNil(t, err)
			require.Contains(t, err.Error(), "certificate signed by unknown authority")
		})
	})
}

func TestCanAcceptSelfSignedCertWithCA(t *testing.T) {
	shared.WithTempFile(func(certFile string) {
		shared.WithTempFile(func(keyFile string) {
			err := shared.MakeSelfSignedCert(certFile, keyFile)
			require.NoError(t, err)

			server, err := shared.MakeServerWithCert(certFile, keyFile, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(200)
			}))
			require.NoError(t, err)
			defer server.Close()

			transport, _, err := NewHTTPTransport(CACertFileOption(certFile))
			require.NoError(t, err)

			client := *http.DefaultClient
			client.Transport = transport
			resp, err := client.Get(server.URL)
			require.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)
		})
	})
}

func TestErrorForNonexistentCertFile(t *testing.T) {
	shared.WithTempFile(func(certFile string) {
		os.Remove(certFile)
		_, _, err := NewHTTPTransport(CACertFileOption(certFile))
		require.Error(t, err)
		require.Contains(t, err.Error(), "Can't read CA certificate file")
	})
}

func TestErrorForCertFileWithBadData(t *testing.T) {
	shared.WithTempFile(func(certFile string) {
		ioutil.WriteFile(certFile, []byte("sorry"), os.ModeAppend)
		_, _, err := NewHTTPTransport(CACertFileOption(certFile))
		require.Error(t, err)
		require.Contains(t, err.Error(), "Invalid CA certificate data")
	})
}

func TestErrorForBadCertData(t *testing.T) {
	_, _, err := NewHTTPTransport(CACertOption([]byte("sorry")))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid CA certificate data")
}

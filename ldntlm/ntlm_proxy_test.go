package ldntlm

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/ldhttp"
)

const (
	username      = "username"
	password      = "password"
	domain        = "domain"
	targetURL     = "http://example.com/test"
	targetServer  = "example.com:80"
	targetURLPath = "/test"
	responseBody  = "hello"
	// The following base64 NTLM message strings/patterns should not be considered authoritative; the exact content will
	// vary depending on the time, the server implementation, etc. We're just verifying that the proxy logic is sending
	// well-formed messages in the order that we expect, and is able to decode a well-formed server response.
	proxyAuthStep1Expected      = "NTLM TlRMTVNTUAABAAAAAZKIoAYABgAoAAAAAAAAAC4AAAAGAbEdAAAAD0RPTUFJTg=="
	proxyAuthStep2Challenge     = "NTLM TlRMTVNTUAACAAAADAAMADAAAAA1gomgZ38cVXpe6WwAAAAAAAAAAEYARgA8AAAAVABFAFMAVABOAFQAAgAMAFQARQBTAFQATgBUAAEADABNAEUATQBCAEUAUgADAB4AbQBlAG0AYgBlAHIALgB0AGUAcwB0AC4AYwBvAG0AAAAAAA=="
	proxyAuthStep3ExpectedRegex = "NTLM TlRMTVNTUAADAAAAAAAAAEAAAAB2AHYAQAAAAAwADAC2AAAAEAAQAMIAAAAUABQA0gAAAAAAAAAAAAAANYK.*AAAAAAgAMAFQARQBTAFQATgBUAAEADABNAEUATQBCAEUAUgADAB4AbQBlAG0AYgBlAHIALgB0AGUAcwB0AC4AYwBvAG0AAAAAAAAAAABUAEUAUwBUAE4AVAB1AHMAZQByAG4AYQBtAGUAZwBvAC0AbgB0AGwAbQBzAHMAcAA="
)

func TestCanConnectToNTLMProxyServer(t *testing.T) {
	httphelpers.WithServer(makeFakeNTLMProxyHandler(), func(server *httptest.Server) {
		factory, err := NewNTLMProxyHTTPClientFactory(server.URL, username, password, domain)
		require.NoError(t, err)
		client := factory()

		resp, err := client.Get(targetURL)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, responseBody, string(body))
	})
}

func TestCanConnectSecurelyToNTLMProxyServerWithSelfSignedCert(t *testing.T) {
	handler := makeFakeNTLMProxyHandler()
	httphelpers.WithSelfSignedServer(handler, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
		factory, err := NewNTLMProxyHTTPClientFactory(server.URL, username, password, domain,
			ldhttp.CACertOption(certData))
		require.NoError(t, err)
		client := factory()

		resp, err := client.Get(targetURL)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, responseBody, string(body))
	})
}

func TestInvalidParameters(t *testing.T) {
	_, err := NewNTLMProxyHTTPClientFactory("", "user", "pass", domain)
	assert.Error(t, err)

	_, err = NewNTLMProxyHTTPClientFactory("http://proxy", "", "pass", domain)
	assert.Error(t, err)

	_, err = NewNTLMProxyHTTPClientFactory("http://proxy", "user", "", domain)
	assert.Error(t, err)

	_, err = NewNTLMProxyHTTPClientFactory("://bad", "user", "pass", domain)
	assert.Error(t, err)

	_, err = NewNTLMProxyHTTPClientFactory("http://proxy", "user", "pass", "domain",
		ldhttp.CACertOption([]byte("not a valid cert")))
	assert.Error(t, err)
}

func makeFakeNTLMProxyHandler() http.Handler {
	step := 0
	// This is an extremely minimal simulation of an NTLM proxy exchange:
	// 1. Client sends CONNECT request, with Proxy-Authorization header containing "negotiate" message.
	//    Server sends 407 response, with Proxy-Authenticate header containing "challenge" message.
	// 2. Client sends CONNECT request, with Proxy-Authorization header containing "authorization" message.
	//    Server sends 200 response.
	// 3. Client sends GET request for target URL.
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		step = step + 1
		expectedMethod := "CONNECT"
		expectedURL := targetServer
		if step == 3 {
			expectedMethod = "GET"
			expectedURL = targetURLPath
		}
		if step < 3 {
			if req.Method != expectedMethod {
				fmt.Printf("Expected %s, got %s for step %d\n", expectedMethod, req.Method, step)
				w.WriteHeader(405)
				return
			}
			if req.RequestURI != expectedURL {
				fmt.Printf("Expected %s, got %s for step %d\n", expectedURL, req.RequestURI, step)
				w.WriteHeader(404)
				return
			}
		}
		proxyAuth := req.Header.Get("Proxy-Authorization")
		badAuth := func() {
			fmt.Printf("Unexpected Proxy-Authorization value: %s\n", proxyAuth)
			w.WriteHeader(401)
		}
		switch step {
		case 1:
			if proxyAuth == proxyAuthStep1Expected {
				w.Header().Set("Proxy-Authenticate", proxyAuthStep2Challenge)
				w.WriteHeader(407)
			} else {
				badAuth()
			}
		case 2:
			if matched, _ := regexp.MatchString(proxyAuthStep3ExpectedRegex, proxyAuth); matched {
				w.WriteHeader(200)
			} else {
				badAuth()
			}
		case 3:
			w.WriteHeader(200)
			w.Write([]byte(responseBody))
		}
	})
}

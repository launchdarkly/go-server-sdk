package ldntlm

import (
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-server-sdk/v7/ldhttp"
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
		body, err := io.ReadAll(resp.Body)
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
		body, err := io.ReadAll(resp.Body)
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
	// This is an extremely minimal simulation of an NTLM proxy exchange:
	// 1. Client sends CONNECT request, with Proxy-Authorization header containing "negotiate" message.
	//    Server sends 407 response, with Proxy-Authenticate header containing "challenge" message.
	// 2. Client sends CONNECT request, with Proxy-Authorization header containing "authorization" message.
	//    Server sends 200 response.
	// 3. Client sends GET request for target URL.
	//
	// NTLM authentication is connection-oriented: steps 2 and 3 must happen on the same connection
	// that received the 407 in step 1. As of Go 1.27.2, net/http.Server closes the connection after
	// responding to a CONNECT request (even a rejection), so the handler hijacks the connection on
	// the first request and speaks raw HTTP/1.1 for the rest of the exchange, as a real NTLM proxy
	// would.
	return http.HandlerFunc(func(w http.ResponseWriter, firstReq *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			fmt.Println("ResponseWriter does not support hijacking")
			w.WriteHeader(500)
			return
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			fmt.Printf("Hijack failed: %s\n", err)
			return
		}
		defer conn.Close()

		send := func(rawResponse string) bool {
			_, err := rw.WriteString(rawResponse)
			if err == nil {
				err = rw.Flush()
			}
			if err != nil {
				fmt.Printf("Error writing response: %s\n", err)
			}
			return err == nil
		}

		req := firstReq
		for step := 1; step <= 3; step++ {
			if step > 1 {
				req, err = http.ReadRequest(rw.Reader)
				if err != nil {
					fmt.Printf("Error reading request for step %d: %s\n", step, err)
					return
				}
			}
			expectedMethod := "CONNECT"
			expectedURL := targetServer
			if step == 3 {
				expectedMethod = "GET"
				expectedURL = targetURLPath
			}
			if req.Method != expectedMethod {
				fmt.Printf("Expected %s, got %s for step %d\n", expectedMethod, req.Method, step)
				send("HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\n\r\n")
				return
			}
			if req.RequestURI != expectedURL {
				fmt.Printf("Expected %s, got %s for step %d\n", expectedURL, req.RequestURI, step)
				send("HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n")
				return
			}
			proxyAuth := req.Header.Get("Proxy-Authorization")
			badAuth := func() {
				fmt.Printf("Unexpected Proxy-Authorization value: %s\n", proxyAuth)
				send("HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n")
			}
			switch step {
			case 1:
				if proxyAuth != proxyAuthStep1Expected {
					badAuth()
					return
				}
				if !send("HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: " +
					proxyAuthStep2Challenge + "\r\nContent-Length: 0\r\n\r\n") {
					return
				}
			case 2:
				if matched, _ := regexp.MatchString(proxyAuthStep3ExpectedRegex, proxyAuth); !matched {
					badAuth()
					return
				}
				// A 2xx response to CONNECT must not carry body-framing headers (RFC 9112 section
				// 6.3); after it, the connection is a tunnel to the target.
				if !send("HTTP/1.1 200 Connection established\r\n\r\n") {
					return
				}
			case 3:
				send(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(responseBody), responseBody))
			}
		}
	})
}

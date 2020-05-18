package internal

import (
	"net/http"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/ldhttp"
)

const defaultHTTPTimeout = 3 * time.Second

// NewHTTPClient creates an HTTP client based on the SDK's configuration.
func NewHTTPClient(timeout time.Duration, options ...ldhttp.TransportOption) http.Client {
	client := http.Client{
		Timeout: timeout,
	}
	if timeout <= 0 {
		client.Timeout = defaultHTTPTimeout
	}
	allOpts := []ldhttp.TransportOption{ldhttp.ConnectTimeoutOption(timeout)}
	allOpts = append(allOpts, options...)
	if transport, _, err := ldhttp.NewHTTPTransport(allOpts...); err == nil {
		client.Transport = transport
	}
	return client
}

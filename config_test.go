package ldclient

import (
	"net/http"
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

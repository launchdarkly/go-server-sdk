package ldclient

import (
	"net/http"
)

type urlAppendingHTTPAdapter string

func (a urlAppendingHTTPAdapter) TransformClient(client http.Client) http.Client {
	ret := client
	ret.Transport = a
	return ret
}

func (a urlAppendingHTTPAdapter) RoundTrip(r *http.Request) (*http.Response, error) {
	req := *r
	req.URL.Path = req.URL.Path + string(a)
	return http.DefaultTransport.RoundTrip(&req)
}

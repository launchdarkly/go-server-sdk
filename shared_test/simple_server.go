package shared_test

import (
	"net/http"
	"net/http/httptest"
)

// Minimal stub server created by NewStubHTTPServer.
type StubHTTPServer struct {
	URL           string
	RequestedURLs []string
	Requests      []*http.Request
	server        *httptest.Server
}

// A parameter for NewStubHTTPServer.
type StubResponse struct {
	Code        int
	Body        string
	ContentType string
}

// NewStubHTTPServer reates a minimal stub server that records requests and always provides the same canned response.
func NewStubHTTPServer(resp StubResponse) *StubHTTPServer {
	s := &StubHTTPServer{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s.Requests = append(s.Requests, req)
		s.RequestedURLs = append(s.RequestedURLs, req.RequestURI)
		if resp.ContentType != "" {
			w.Header().Add("Content-Type", resp.ContentType)
		}
		if resp.Code > 0 {
			w.WriteHeader(resp.Code)
		} else {
			w.WriteHeader(200)
		}
		if resp.Body != "" {
			_, _ = w.Write([]byte(resp.Body))
		}
	}))
	s.URL = s.server.URL
	return s
}

// Close calls Close() on the underlying test server.
func (s *StubHTTPServer) Close() {
	s.server.Close()
}

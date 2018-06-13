package ldclient

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var nullHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

func TestPollingProcessor_ClosingItShouldNotBlock(t *testing.T) {
	server := httptest.NewServer(nullHandler)
	defer server.Close()
	cfg := Config{
		Logger:       log.New(ioutil.Discard, "", 0),
		PollInterval: time.Minute,
	}
	req := newFakeRequestor(server, cfg)
	p := newPollingProcessor(cfg, req)

	p.Close()

	closeWhenReady := make(chan struct{})
	p.Start(closeWhenReady)

	select {
	case <-closeWhenReady:
	case <-time.After(time.Second):
		assert.Fail(t, "Start a closed processor shouldn't block")
	}
}

func TestPollingProcessor_401ShouldNotBlock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	defer ts.Close()

	cfg := Config{
		Logger:       log.New(ioutil.Discard, "", 0),
		PollInterval: time.Minute,
		BaseUri:      ts.URL,
	}
	req := newFakeRequestor(ts, cfg)
	p := newPollingProcessor(cfg, req)

	closeWhenReady := make(chan struct{})
	p.Start(closeWhenReady)

	select {
	case <-closeWhenReady:
	case <-time.After(time.Second):
		assert.Fail(t, "Receiving 401 shouldn't block")
	}
}

func newFakeRequestor(server *httptest.Server, config Config) *requestor {
	httpRequestor := requestor{
		sdkKey:     "fake",
		httpClient: http.DefaultClient,
		config:     config,
	}

	return &httpRequestor
}

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

func TestStreamProcessor_401ShouldNotBlock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	cfg := Config{
		StreamUri: ts.URL,
		Logger:    log.New(ioutil.Discard, "", 0),
	}

	sp := newStreamProcessor("sdkKey", cfg, nil)

	closeWhenReady := make(chan struct{})

	sp.subscribe(closeWhenReady)

	select {
	case <-closeWhenReady:
	case <-time.After(time.Second):
		assert.Fail(t, "Receiving 401 shouldn't block")
	}
}

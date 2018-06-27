package ldclient

import (
	"fmt"
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

func TestPollingProcessor_Initialization(t *testing.T) {
	polls := make(chan struct{}, 2)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/sdk/latest-all", r.URL.Path)
		w.Write([]byte(`{"flags": {"my-flag": {"key": "my-flag", "version": 2}}, "segments": {"my-segment": {"key": "my-segment", "version": 3}}}`))
		if len(polls) < cap(polls) {
			polls <- struct{}{}
		}
	}))

	defer ts.Close()
	defer ts.CloseClientConnections()

	store := NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0))

	cfg := Config{
		FeatureStore: store,
		Logger:       log.New(ioutil.Discard, "", 0),
		PollInterval: time.Millisecond,
		BaseUri:      ts.URL,
	}
	req := newFakeRequestor(ts, cfg)
	p := newPollingProcessor(cfg, req)

	closeWhenReady := make(chan struct{})
	p.Start(closeWhenReady)

	select {
	case <-closeWhenReady:
	case <-time.After(time.Second):
		assert.Fail(t, "Failed to initialize")
		return
	}

	flag, err := store.Get(Features, "my-flag")
	if assert.NoError(t, err) {
		assert.Equal(t, 2, flag.GetVersion())
	}

	segment, err := store.Get(Segments, "my-segment")
	if assert.NoError(t, err) {
		assert.Equal(t, 3, segment.GetVersion())
	}

	for i := 0; i < 2; i++ {
		select {
		case <-polls:
		case <-time.After(time.Second):
			assert.Fail(t, "Expected 2 polls but only got %d", i)
			return
		}
	}
}

func TestPollingProcessorRequestResponseCodes(t *testing.T) {
	specs := []struct {
		statusCode  int
		recoverable bool
	}{
		{400, false},
		{401, false},
		{403, false},
		{405, false},
		{408, true},
		{429, true},
		{500, true},
	}

	for _, tt := range specs {
		t.Run(fmt.Sprintf("status %d, recoverable %v", tt.statusCode, tt.recoverable), func(t *testing.T) {
			polls := make(chan struct{}, 2)

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if len(polls) < cap(polls) {
					polls <- struct{}{}
				}
				w.WriteHeader(tt.statusCode)
			}))

			defer ts.Close()
			defer ts.CloseClientConnections()

			cfg := Config{
				Logger:       log.New(ioutil.Discard, "", 0),
				PollInterval: time.Millisecond * 10,
				BaseUri:      ts.URL,
			}
			req := newFakeRequestor(ts, cfg)
			p := newPollingProcessor(cfg, req)
			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			if tt.recoverable {
				// wait for two polls
				for i := 0; i < 2; i++ {
					select {
					case <-polls:
						t.Logf("Got poll attempt %d/2", i+1)
					case <-closeWhenReady:
						assert.Fail(t, "should not report ready")
						break
					case <-time.After(time.Second * 3):
						assert.Fail(t, "failed to retry")
						break
					}
				}
			} else {
				select {
				case <-closeWhenReady:
					assert.Len(t, polls, 1) // should be ready after a single attempt
					assert.False(t, p.Initialized())
				case <-time.After(time.Second):
					assert.Fail(t, "channel was not closed immediately")
				}
			}
		})
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

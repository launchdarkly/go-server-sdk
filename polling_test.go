package ldclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v4/ldlog"

	"github.com/stretchr/testify/assert"
)

var nullHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

func TestPollingProcessorClosingItShouldNotBlock(t *testing.T) {
	server := httptest.NewServer(nullHandler)
	defer server.Close()
	cfg := Config{
		Loggers:      ldlog.NewDisabledLoggers(),
		PollInterval: time.Minute,
		BaseUri:      server.URL,
	}
	req := newRequestor("fake", cfg, nil)
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

func TestPollingProcessorInitialization(t *testing.T) {
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

	cfg := Config{
		Loggers:      ldlog.NewDisabledLoggers(),
		PollInterval: time.Millisecond,
		BaseUri:      ts.URL,
	}
	store, _ := NewInMemoryFeatureStoreFactory()(cfg)
	cfg.FeatureStore = store
	req := newRequestor("fake", cfg, nil)
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
		{400, true},
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
				Loggers:      ldlog.NewDisabledLoggers(),
				PollInterval: time.Millisecond * 10,
				BaseUri:      ts.URL,
			}
			req := newRequestor("fake", cfg, nil)
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

func TestPollingProcessorUsesHTTPClientFactory(t *testing.T) {
	polledURLs := make(chan string, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polledURLs <- r.URL.Path
		w.Write([]byte(`{"flags": {"my-flag": {"key": "my-flag", "version": 2}}, "segments": {}}`))
	}))
	defer ts.Close()
	defer ts.CloseClientConnections()

	cfg := Config{
		Loggers:           ldlog.NewDisabledLoggers(),
		PollInterval:      time.Minute * 30,
		BaseUri:           ts.URL,
		HTTPClientFactory: urlAppendingHTTPClientFactory("/transformed"),
	}
	store, _ := NewInMemoryFeatureStoreFactory()(cfg)
	cfg.FeatureStore = store
	req := newRequestor("fake", cfg, nil)

	p := newPollingProcessor(cfg, req)
	defer p.Close()
	closeWhenReady := make(chan struct{})
	p.Start(closeWhenReady)

	polledURL := <-polledURLs

	assert.Equal(t, "/sdk/latest-all/transformed", polledURL)
}

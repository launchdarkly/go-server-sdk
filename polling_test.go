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
	shared "gopkg.in/launchdarkly/go-server-sdk.v4/shared_test"
)

var nullHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

func TestPollingProcessorClosingItShouldNotBlock(t *testing.T) {
	server := httptest.NewServer(nullHandler)
	defer server.Close()
	cfg := Config{
		Loggers:      shared.NullLoggers(),
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
	data := shared.SDKData{
		FlagsData:    []byte(`{"my-flag": {"key": "my-flag", "version": 2}}`),
		SegmentsData: []byte(`{"my-segment": {"key": "my-segment", "version": 3}}`),
	}
	pollHandler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewPollingServiceHandler(data))
	ts := httptest.NewServer(pollHandler)

	defer ts.Close()
	defer ts.CloseClientConnections()

	store := NewInMemoryFeatureStore(log.New(ioutil.Discard, "", 0))

	cfg := Config{
		FeatureStore: store,
		Loggers:      shared.NullLoggers(),
		PollInterval: time.Millisecond,
		BaseUri:      ts.URL,
	}
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
		case <-requestsCh:
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
			handler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewHTTPHandlerReturningStatus(tt.statusCode))
			ts := httptest.NewServer(handler)

			defer ts.Close()
			defer ts.CloseClientConnections()

			cfg := Config{
				Loggers:      shared.NullLoggers(),
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
					case <-requestsCh:
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
					assert.Len(t, requestsCh, 1) // should be ready after a single attempt
					assert.False(t, p.Initialized())
				case <-time.After(time.Second):
					assert.Fail(t, "channel was not closed immediately")
				}
			}
		})
	}
}

func TestPollingProcessorUsesHTTPClientFactory(t *testing.T) {
	data := shared.SDKData{
		FlagsData: []byte(`{"my-flag": {"key": "my-flag", "version": 2}}`),
	}
	pollHandler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewPollingServiceHandler(data))
	ts := httptest.NewServer(pollHandler)
	defer ts.Close()
	defer ts.CloseClientConnections()

	store := NewInMemoryFeatureStore(nil)

	cfg := Config{
		FeatureStore:      store,
		Loggers:           shared.NullLoggers(),
		PollInterval:      time.Minute * 30,
		BaseUri:           ts.URL,
		HTTPClientFactory: urlAppendingHTTPClientFactory("/transformed"),
	}
	req := newRequestor("fake", cfg, nil)

	p := newPollingProcessor(cfg, req)
	defer p.Close()
	closeWhenReady := make(chan struct{})
	p.Start(closeWhenReady)

	r := <-requestsCh

	assert.Equal(t, "/sdk/latest-all/transformed", r.Request.URL.Path)
}

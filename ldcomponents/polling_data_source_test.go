package ldcomponents

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var nullHandler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})

func TestPollingProcessorClosingItShouldNotBlock(t *testing.T) {
	server := httptest.NewServer(nullHandler)
	defer server.Close()
	req := newRequestor(basicClientContext(), nil, server.URL)
	p := newPollingProcessor(basicClientContext(), makeInMemoryDataStore(), req, time.Minute)

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

	store := makeInMemoryDataStore()
	p, err := PollingDataSource().BaseURI(ts.URL).forcePollInterval(time.Millisecond*10).CreateDataSource(basicClientContext(), store)
	require.NoError(t, err)

	closeWhenReady := make(chan struct{})
	p.Start(closeWhenReady)

	select {
	case <-closeWhenReady:
	case <-time.After(time.Second):
		assert.Fail(t, "Failed to initialize")
		return
	}

	flag, err := store.Get(interfaces.DataKindFeatures(), "my-flag")
	if assert.NoError(t, err) {
		assert.Equal(t, 2, flag.GetVersion())
	}

	segment, err := store.Get(interfaces.DataKindSegments(), "my-segment")
	if assert.NoError(t, err) {
		assert.Equal(t, 3, segment.GetVersion())
	}

	for i := 0; i < 2; i++ {
		select {
		case <-polls:
		case <-time.After(time.Second):
			assert.Fail(t, "Expected 2 polls", "but only got %d", i)
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

			req := newRequestor(basicClientContext(), nil, ts.URL)
			p := newPollingProcessor(basicClientContext(), makeInMemoryDataStore(), req, time.Millisecond*10)
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

	httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
	context := interfaces.NewClientContext(testSdkKey, nil, httpClientFactory, ldlog.NewDisabledLoggers())
	req := newRequestor(context, nil, ts.URL)

	p := newPollingProcessor(context, makeInMemoryDataStore(), req, time.Minute*30)
	defer p.Close()
	closeWhenReady := make(chan struct{})
	p.Start(closeWhenReady)

	polledURL := <-polledURLs

	assert.Equal(t, "/sdk/latest-all/transformed", polledURL)
}

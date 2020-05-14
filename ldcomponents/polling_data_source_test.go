package ldcomponents

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/httphelpers"
	"github.com/launchdarkly/go-test-helpers/ldservices"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollingProcessorClosingItShouldNotBlock(t *testing.T) {
	handler := ldservices.ServerSidePollingServiceHandler(ldservices.NewServerSDKData())
	httphelpers.WithServer(handler, func(server *httptest.Server) {
		withDataSourceTestParams(func(params dataSourceTestParams) {
			req := newRequestor(basicClientContext(), nil, server.URL)
			p := newPollingProcessor(basicClientContext(), params.store, req, time.Minute)

			p.Close()

			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			select {
			case <-closeWhenReady:
			case <-time.After(time.Second):
				assert.Fail(t, "Start a closed processor shouldn't block")
			}
		})
	})
}

func TestPollingProcessorInitialization(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2)).Segments(ldservices.FlagOrSegment("my-segment", 3))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		withDataSourceTestParams(func(params dataSourceTestParams) {
			p, err := PollingDataSource().
				BaseURI(ts.URL).
				forcePollInterval(time.Millisecond*10).
				CreateDataSource(basicClientContext(), params.store, params.dataStoreStatusProvider)
			require.NoError(t, err)
			defer p.Close()

			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			select {
			case <-closeWhenReady:
			case <-time.After(time.Second):
				assert.Fail(t, "Failed to initialize")
				return
			}

			params.waitForInit(t, data)

			for i := 0; i < 2; i++ {
				select {
				case <-requestsCh:
				case <-time.After(time.Second):
					assert.Fail(t, "Expected 2 polls", "but only got %d", i)
					return
				}
			}
		})
	})
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
			handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(tt.statusCode))
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				withDataSourceTestParams(func(params dataSourceTestParams) {
					req := newRequestor(basicClientContext(), nil, ts.URL)
					p := newPollingProcessor(basicClientContext(), params.store, req, time.Millisecond*10)
					defer p.Close()
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
			})
		})
	}
}

func TestPollingProcessorUsesHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		withDataSourceTestParams(func(params dataSourceTestParams) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			context := interfaces.NewClientContext(testSdkKey, nil, httpClientFactory, sharedtest.NewTestLoggers())
			req := newRequestor(context, nil, ts.URL)

			p := newPollingProcessor(context, params.store, req, time.Minute*30)
			defer p.Close()
			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			r := <-requestsCh

			assert.Equal(t, "/sdk/latest-all/transformed", r.Request.URL.Path)
		})
	})
}

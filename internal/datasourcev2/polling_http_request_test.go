package datasourcev2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV2RequestorImplRequestAll(t *testing.T) {
	testWithFilters(t, func(t *testing.T, filter filterTest) {
		t.Run("success", func(t *testing.T) {
			flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).SingleVariation(ldvalue.Bool(true)).Build()
			segment := ldbuilders.NewSegmentBuilder("segmentkey").Version(1).Build()
			expectedData := ldservicesv2.NewServerSDKData().Flags(flag).Segments(segment).ToInitializerPayload(subsystems.NewSelector("test-state", 1))
			handler, requestsCh := httphelpers.RecordingHandler(
				ldservices.ServerSidePollingV2ServiceHandler(expectedData),
			)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(sharedtest.BasicClientContext(), nil, ts.URL, filter.key)

				changeSet, _, err := r.Request(context.Background(), subsystems.NoSelector())

				assert.NoError(t, err)
				assert.Equal(t, subsystems.IntentTransferFull, changeSet.IntentCode())
				assert.Equal(t, "test-state", changeSet.Selector().State())
				assert.Equal(t, 1, changeSet.Selector().Version())

				req := <-requestsCh
				assert.Equal(t, "/sdk/poll", req.Request.URL.Path)
				assert.Equal(t, filter.query, req.Request.URL.RawQuery)
			})
		})

		t.Run("HTTP error response", func(t *testing.T) {
			handler := httphelpers.HandlerWithStatus(500)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(sharedtest.BasicClientContext(), nil, ts.URL, filter.key)

				changeSet, _, err := r.Request(context.Background(), subsystems.NoSelector())

				assert.Error(t, err)
				if he, ok := err.(httpStatusError); assert.True(t, ok) {
					assert.Equal(t, 500, he.Code)
				}
				assert.Nil(t, changeSet)
			})
		})

		t.Run("network error", func(t *testing.T) {
			var closedServerURL string
			handler := httphelpers.HandlerWithJSONResponse(ldservices.NewServerSDKData(), nil)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				closedServerURL = ts.URL
			})
			r := newPollingRequester(sharedtest.BasicClientContext(), nil, closedServerURL, filter.key)

			changeSet, _, err := r.Request(context.Background(), subsystems.NoSelector())

			assert.Error(t, err)
			assert.Nil(t, changeSet)
		})

		t.Run("malformed data", func(t *testing.T) {
			handler := httphelpers.HandlerWithResponse(200, nil, []byte("{"))
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(sharedtest.BasicClientContext(), nil, ts.URL, filter.key)

				changeSet, _, err := r.Request(context.Background(), subsystems.NoSelector())

				require.Error(t, err)
				_, ok := err.(malformedJSONError)
				assert.True(t, ok)
				assert.Nil(t, changeSet)
			})
		})

		t.Run("malformed base URI", func(t *testing.T) {
			r := newPollingRequester(sharedtest.BasicClientContext(), nil, "::::", filter.key)

			changeSet, _, err := r.Request(context.Background(), subsystems.NoSelector())

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing protocol scheme")
			assert.Nil(t, changeSet)
		})

		t.Run("sends configured headers", func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("my-header", "my-value")
			expectedData := ldservicesv2.NewServerSDKData().ToInitializerPayload(subsystems.NewSelector("test-state", 1))
			handler, requestsCh := httphelpers.RecordingHandler(
				ldservices.ServerSidePollingV2ServiceHandler(expectedData),
			)
			httpConfig := subsystems.HTTPConfiguration{DefaultHeaders: headers}
			ldcontext := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(ldcontext, nil, ts.URL, filter.key)

				_, _, err := r.Request(context.Background(), subsystems.NoSelector())
				assert.NoError(t, err)

				req := <-requestsCh
				assert.Equal(t, "my-value", req.Request.Header.Get("my-header"))
			})
		})

		t.Run("logs debug message", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			mockLog.Loggers.SetMinLevel(ldlog.Debug)
			ldcontext := sharedtest.NewTestContext(sharedtest.TestSDKKey, nil, &subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
			expectedData := ldservicesv2.NewServerSDKData().ToInitializerPayload(subsystems.NewSelector("test-state", 1))
			handler, _ := httphelpers.RecordingHandler(
				ldservices.ServerSidePollingV2ServiceHandler(expectedData),
			)

			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(ldcontext, nil, ts.URL, filter.key)

				_, _, err := r.Request(context.Background(), subsystems.NoSelector())
				assert.NoError(t, err)

				assert.Equal(t, []string{"Polling LaunchDarkly for feature flag updates"},
					mockLog.GetOutput(ldlog.Debug))
			})
		})
	})
}

func TestV2RequestorImplCaching(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).SingleVariation(ldvalue.Bool(true)).Build()
	selector := subsystems.NewSelector("test-state", 1)
	expectedData := ldservicesv2.NewServerSDKData().Flags(flag).ToInitializerPayload(selector)
	upToDateData := ldservicesv2.NewServerSDKData().ToInitializerPayload(selector)

	testWithFilters(t, func(t *testing.T, filter filterTest) {
		handler, requestsCh := httphelpers.RecordingHandler(
			httphelpers.SequentialHandler(
				ldservices.ServerSidePollingV2ServiceHandler(expectedData), // the first request returns the full data and is cached with a key of /sdk/poll
				ldservices.ServerSidePollingV2ServiceHandler(upToDateData), // the second request returns a FDv2 response with no changes and is cached with a key of /sdk/poll?basis=test-state
				httphelpers.HandlerWithStatus(304),                         // the third request returns a 304 Not Modified response, which the cache can use to determine that it should return the (second) cached data
			),
		)
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			r := newPollingRequester(sharedtest.BasicClientContext(), nil, ts.URL, filter.key)

			changeSet1, _, err1 := r.Request(context.Background(), subsystems.NoSelector())

			assert.NoError(t, err1)
			assert.NotNil(t, changeSet1)

			req1 := <-requestsCh
			assert.Equal(t, "/sdk/poll", req1.Request.URL.Path)
			assert.Equal(t, filter.key, req1.Request.URL.Query().Get("filter"))
			assert.False(t, req1.Request.URL.Query().Has("basis"))

			assert.Equal(t, "", req1.Request.Header.Get("If-None-Match"))

			changeSet2, _, err2 := r.Request(context.Background(), selector)

			assert.NoError(t, err2)
			assert.Equal(t, subsystems.IntentNone, changeSet2.IntentCode())
			assert.Len(t, changeSet2.Changes(), 0)

			req2 := <-requestsCh
			assert.Equal(t, "/sdk/poll", req2.Request.URL.Path)
			assert.Equal(t, filter.key, req2.Request.URL.Query().Get("filter"))
			assert.Equal(t, "test-state", req2.Request.URL.Query().Get("basis"))

			assert.Equal(t, "", req2.Request.Header.Get("If-None-Match"))

			changeSet3, _, err3 := r.Request(context.Background(), selector)

			assert.NoError(t, err3)
			assert.Equal(t, subsystems.IntentNone, changeSet3.IntentCode())
			assert.Len(t, changeSet3.Changes(), 0)

			req3 := <-requestsCh
			assert.Equal(t, "/sdk/poll", req3.Request.URL.Path)
			assert.Equal(t, filter.key, req3.Request.URL.Query().Get("filter"))
			assert.Equal(t, "test-state", req3.Request.URL.Query().Get("basis"))

			assert.Equal(t, "", req3.Request.Header.Get("If-None-Match"))
		})
	})
}

func TestV2RequestorImplCanUseCustomHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
	httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
	ldcontext := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		r := newPollingRequester(ldcontext, nil, ts.URL, "")

		_, _, _ = r.Request(context.Background(), subsystems.NoSelector())

		req := <-requestsCh

		assert.Equal(t, "/sdk/poll/transformed", req.Request.URL.Path)
	})
}

func TestV2RequestorImplCanAppendsFilterParameter(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))

	testWithFilters(t, func(t *testing.T, filter filterTest) {
		httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
			r := newPollingRequester(sharedtest.BasicClientContext(), nil, ts.URL, filter.key)

			_, _, _ = r.Request(context.Background(), subsystems.NoSelector())

			req := <-requestsCh

			assert.Equal(t, filter.query, req.Request.URL.RawQuery)
		})
	})
}

func TestV2RequestorImplCanAppendsBasis(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))

	testWithFilters(t, func(t *testing.T, filter filterTest) {
		httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
			r := newPollingRequester(sharedtest.BasicClientContext(), nil, ts.URL, filter.key)

			_, _, _ = r.Request(context.Background(), subsystems.NewSelector("test-state", 1))

			req := <-requestsCh

			assert.Equal(t, "test-state", req.Request.URL.Query().Get("basis"))
		})
	})
}

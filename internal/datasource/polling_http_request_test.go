package datasource

import (
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

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestorImplRequestAll(t *testing.T) {
	testWithFilters(t, func(t *testing.T, filter filterTest) {
		t.Run("success", func(t *testing.T) {
			flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).SingleVariation(ldvalue.Bool(true)).Build()
			segment := ldbuilders.NewSegmentBuilder("segmentkey").Version(1).Build()
			override := ldbuilders.NewConfigOverrideBuilder("overridekey").Version(1).Build()
			metric := ldbuilders.NewMetricBuilder("metrickey").Version(1).Build()
			expectedData := sharedtest.NewDataSetBuilder().Flags(flag).Segments(segment).ConfigOverrides(override).Metrics(metric)
			handler, requestsCh := httphelpers.RecordingHandler(
				ldservices.ServerSidePollingServiceHandler(expectedData.ToServerSDKData()),
			)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(basicClientContext(), nil, ts.URL, filter.key)

				data, cached, err := r.Request()

				assert.NoError(t, err)
				assert.False(t, cached)

				assert.Equal(t, sharedtest.NormalizeDataSet(expectedData.Build()), sharedtest.NormalizeDataSet(data))

				req := <-requestsCh
				assert.Equal(t, "/sdk/latest-all", req.Request.URL.Path)
				assert.Equal(t, filter.query, req.Request.URL.RawQuery)
			})
		})

		t.Run("HTTP error response", func(t *testing.T) {
			handler := httphelpers.HandlerWithStatus(500)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(basicClientContext(), nil, ts.URL, filter.key)

				data, cached, err := r.Request()

				assert.Error(t, err)
				if he, ok := err.(httpStatusError); assert.True(t, ok) {
					assert.Equal(t, 500, he.Code)
				}
				assert.False(t, cached)
				assert.Nil(t, data)
			})

		})

		t.Run("network error", func(t *testing.T) {
			var closedServerURL string
			handler := httphelpers.HandlerWithJSONResponse(ldservices.NewServerSDKData(), nil)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				closedServerURL = ts.URL
			})
			r := newPollingRequester(basicClientContext(), nil, closedServerURL, filter.key)

			data, cached, err := r.Request()

			assert.Error(t, err)
			assert.False(t, cached)
			assert.Nil(t, data)
		})

		t.Run("malformed data", func(t *testing.T) {
			handler := httphelpers.HandlerWithResponse(200, nil, []byte("{"))
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(basicClientContext(), nil, ts.URL, filter.key)

				data, cached, err := r.Request()

				require.Error(t, err)
				_, ok := err.(malformedJSONError)
				assert.True(t, ok)
				assert.False(t, cached)
				assert.Nil(t, data)
			})
		})

		t.Run("malformed base URI", func(t *testing.T) {
			r := newPollingRequester(basicClientContext(), nil, "::::", filter.key)

			data, cached, err := r.Request()

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing protocol scheme")
			assert.False(t, cached)
			assert.Nil(t, data)
		})

		t.Run("sends configured headers", func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("my-header", "my-value")
			handler, requestsCh := httphelpers.RecordingHandler(
				httphelpers.HandlerWithJSONResponse(ldservices.NewServerSDKData(), nil),
			)
			httpConfig := subsystems.HTTPConfiguration{DefaultHeaders: headers}
			context := sharedtest.NewTestContext(testSDKKey, &httpConfig, nil)

			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(context, nil, ts.URL, filter.key)

				_, _, err := r.Request()
				assert.NoError(t, err)

				req := <-requestsCh
				assert.Equal(t, "my-value", req.Request.Header.Get("my-header"))
			})
		})

		t.Run("logs debug message", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			mockLog.Loggers.SetMinLevel(ldlog.Debug)
			context := sharedtest.NewTestContext(testSDKKey, nil, &subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
			handler := httphelpers.HandlerWithJSONResponse(ldservices.NewServerSDKData(), nil)

			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				r := newPollingRequester(context, nil, ts.URL, filter.key)

				_, _, err := r.Request()
				assert.NoError(t, err)

				assert.Equal(t, []string{"Polling LaunchDarkly for feature flag updates"},
					mockLog.GetOutput(ldlog.Debug))
			})
		})
	})
}

func TestRequestorImplCaching(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).SingleVariation(ldvalue.Bool(true)).Build()
	expectedData := sharedtest.NewDataSetBuilder().Flags(flag)
	etag := "123"

	testWithFilters(t, func(t *testing.T, filter filterTest) {
		handler, requestsCh := httphelpers.RecordingHandler(
			httphelpers.SequentialHandler(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("ETag", etag)
					w.Header().Set("Cache-Control", "max-age=0")
					ldservices.ServerSidePollingServiceHandler(expectedData.ToServerSDKData()).ServeHTTP(w, r)
				}),
				httphelpers.HandlerWithStatus(304),
			),
		)
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			r := newPollingRequester(basicClientContext(), nil, ts.URL, filter.key)

			data1, cached1, err1 := r.Request()

			assert.NoError(t, err1)
			assert.False(t, cached1)
			assert.Equal(t, sharedtest.NormalizeDataSet(expectedData.Build()), sharedtest.NormalizeDataSet(data1))

			req1 := <-requestsCh
			assert.Equal(t, "/sdk/latest-all", req1.Request.URL.Path)
			assert.Equal(t, filter.query, req1.Request.URL.RawQuery)

			assert.Equal(t, "", req1.Request.Header.Get("If-None-Match"))

			data2, cached2, err2 := r.Request()

			assert.NoError(t, err2)
			assert.True(t, cached2)
			assert.Nil(t, data2) // for cached data, requestAll doesn't bother parsing the body

			req2 := <-requestsCh
			assert.Equal(t, "/sdk/latest-all", req2.Request.URL.Path)
			assert.Equal(t, filter.query, req1.Request.URL.RawQuery)

			assert.Equal(t, etag, req2.Request.Header.Get("If-None-Match"))
		})
	})
}

func TestRequestorImplCanUseCustomHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
	httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
	context := sharedtest.NewTestContext(testSDKKey, &httpConfig, nil)

	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		r := newPollingRequester(context, nil, ts.URL, "")

		_, _, _ = r.Request()

		req := <-requestsCh

		assert.Equal(t, "/sdk/latest-all/transformed", req.Request.URL.Path)
	})

}

func TestRequestorImplCanAppendsFilterParameter(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))

	testWithFilters(t, func(t *testing.T, filter filterTest) {
		httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
			r := newPollingRequester(basicClientContext(), nil, ts.URL, filter.key)

			_, _, _ = r.Request()

			req := <-requestsCh

			assert.Equal(t, filter.query, req.Request.URL.RawQuery)
		})
	})
}

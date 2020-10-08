package datasource

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/ldservices"
)

// this mock is not used in the tests in this file; it's used in the polling/streaming data source tests
type mockRequestor struct {
	requestAllRespCh      chan mockRequestAllResponse
	requestResourceRespCh chan mockRequestResourceResponse
	pollsCh               chan struct{}
	closerCh              chan struct{}
}

type mockRequestAllResponse struct {
	data   allData
	cached bool
	err    error
}

type mockRequestResourceResponse struct {
	item ldstoretypes.ItemDescriptor
	err  error
}

func newMockRequestor() *mockRequestor {
	return &mockRequestor{
		requestAllRespCh: make(chan mockRequestAllResponse, 100),
		pollsCh:          make(chan struct{}, 100),
		closerCh:         make(chan struct{}),
	}
}

func (r *mockRequestor) Close() {
	close(r.closerCh)
}

func (r *mockRequestor) requestAll() (allData, bool, error) {
	select {
	case resp := <-r.requestAllRespCh:
		r.pollsCh <- struct{}{}
		return resp.data, resp.cached, resp.err
	case <-r.closerCh:
		return allData{}, false, nil
	}
}

func TestRequestorImplRequestAll(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).SingleVariation(ldvalue.Bool(true)).Build()
		segment := ldbuilders.NewSegmentBuilder("segmentkey").Version(1).Build()
		expectedData := allData{
			Flags:    map[string]*ldmodel.FeatureFlag{flag.Key: &flag},
			Segments: map[string]*ldmodel.Segment{segment.Key: &segment},
		}
		handler, requestsCh := httphelpers.RecordingHandler(
			ldservices.ServerSidePollingServiceHandler(expectedData),
		)
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			r := newRequestorImpl(basicClientContext(), nil, ts.URL)

			data, cached, err := r.requestAll()

			assert.NoError(t, err)
			assert.False(t, cached)

			// Comparing the JSON serializations here rather than directly comparing the flag/segment structs, because
			// flags deserialized from JSON will always have [] instead of nil for an empty slice, whereas ldbuilders
			// can leave fields nil.
			sharedtest.AssertValuesJSONEqual(t, expectedData, data)

			req := <-requestsCh
			assert.Equal(t, "/sdk/latest-all", req.Request.URL.String())
		})
	})

	t.Run("HTTP error response", func(t *testing.T) {
		handler := httphelpers.HandlerWithStatus(500)
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			r := newRequestorImpl(basicClientContext(), nil, ts.URL)

			data, cached, err := r.requestAll()

			assert.Error(t, err)
			if he, ok := err.(httpStatusError); assert.True(t, ok) {
				assert.Equal(t, 500, he.Code)
			}
			assert.False(t, cached)
			assert.Equal(t, allData{}, data)
		})
	})

	t.Run("network error", func(t *testing.T) {
		var closedServerURL string
		handler := httphelpers.HandlerWithJSONResponse(ldservices.NewServerSDKData(), nil)
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			closedServerURL = ts.URL
		})
		r := newRequestorImpl(basicClientContext(), nil, closedServerURL)

		data, cached, err := r.requestAll()

		assert.Error(t, err)
		assert.False(t, cached)
		assert.Equal(t, allData{}, data)
	})

	t.Run("malformed data", func(t *testing.T) {
		handler := httphelpers.HandlerWithResponse(200, nil, []byte("{"))
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			r := newRequestorImpl(basicClientContext(), nil, ts.URL)

			data, cached, err := r.requestAll()

			require.Error(t, err)
			if _, ok := err.(malformedJSONError); assert.True(t, ok) {
				assert.Contains(t, err.Error(), "unexpected end of JSON input")
			}
			assert.False(t, cached)
			assert.Equal(t, allData{}, data)
		})
	})

	t.Run("malformed base URI", func(t *testing.T) {
		r := newRequestorImpl(basicClientContext(), nil, "::::")

		data, cached, err := r.requestAll()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing protocol scheme")
		assert.False(t, cached)
		assert.Equal(t, allData{}, data)
	})

	t.Run("sends configured headers", func(t *testing.T) {
		headers := make(http.Header)
		headers.Set("my-header", "my-value")
		handler, requestsCh := httphelpers.RecordingHandler(
			httphelpers.HandlerWithJSONResponse(ldservices.NewServerSDKData(), nil),
		)
		httpConfig := internal.HTTPConfigurationImpl{DefaultHeaders: headers}
		context := sharedtest.NewTestContext(testSDKKey, httpConfig, sharedtest.TestLoggingConfig())

		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			r := newRequestorImpl(context, nil, ts.URL)

			_, _, err := r.requestAll()
			assert.NoError(t, err)

			req := <-requestsCh
			assert.Equal(t, "my-value", req.Request.Header.Get("my-header"))
		})
	})

	t.Run("logs debug message", func(t *testing.T) {
		mockLog := ldlogtest.NewMockLog()
		mockLog.Loggers.SetMinLevel(ldlog.Debug)
		logConfig := internal.LoggingConfigurationImpl{Loggers: mockLog.Loggers}
		context := sharedtest.NewTestContext(testSDKKey, sharedtest.TestHTTPConfig(), logConfig)
		handler := httphelpers.HandlerWithJSONResponse(ldservices.NewServerSDKData(), nil)

		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			r := newRequestorImpl(context, nil, ts.URL)

			_, _, err := r.requestAll()
			assert.NoError(t, err)

			assert.Equal(t, []string{"Polling LaunchDarkly for feature flag updates"},
				mockLog.GetOutput(ldlog.Debug))
		})
	})
}

func TestRequestorImplCaching(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).SingleVariation(ldvalue.Bool(true)).Build()
	expectedData := allData{
		Flags: map[string]*ldmodel.FeatureFlag{flag.Key: &flag},
	}
	etag := "123"
	handler, requestsCh := httphelpers.RecordingHandler(
		httphelpers.SequentialHandler(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("ETag", etag)
				w.Header().Set("Cache-Control", "max-age=0")
				ldservices.ServerSidePollingServiceHandler(expectedData).ServeHTTP(w, r)
			}),
			httphelpers.HandlerWithStatus(304),
		),
	)
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		r := newRequestorImpl(basicClientContext(), nil, ts.URL)

		data1, cached1, err1 := r.requestAll()

		assert.NoError(t, err1)
		assert.False(t, cached1)
		assert.Equal(t, expectedData, data1)

		req1 := <-requestsCh
		assert.Equal(t, "/sdk/latest-all", req1.Request.URL.String())
		assert.Equal(t, "", req1.Request.Header.Get("If-None-Match"))

		data2, cached2, err2 := r.requestAll()

		assert.NoError(t, err2)
		assert.True(t, cached2)
		assert.Equal(t, allData{}, data2) // for cached data, requestAll doesn't bother parsing the body

		req2 := <-requestsCh
		assert.Equal(t, "/sdk/latest-all", req2.Request.URL.String())
		assert.Equal(t, etag, req2.Request.Header.Get("If-None-Match"))
	})
}

func TestRequestorImplCanUseCustomHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
	httpConfig := internal.HTTPConfigurationImpl{HTTPClientFactory: httpClientFactory}
	context := sharedtest.NewTestContext(testSDKKey, httpConfig, sharedtest.TestLoggingConfig())

	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		r := newRequestorImpl(context, nil, ts.URL)

		_, _, _ = r.requestAll()

		req := <-requestsCh

		assert.Equal(t, "/sdk/latest-all/transformed", req.Request.URL.Path)
	})
}

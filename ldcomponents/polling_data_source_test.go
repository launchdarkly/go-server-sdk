package ldcomponents

import (
	"errors"
	"fmt"
	"net/http"
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
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			req := newRequestor(basicClientContext(), nil, server.URL)
			p := newPollingProcessor(basicClientContext(), dataSourceUpdates, req, time.Minute)

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
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			p, err := PollingDataSource().
				BaseURI(ts.URL).
				forcePollInterval(time.Millisecond*10).
				CreateDataSource(basicClientContext(), dataSourceUpdates)
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

			dataSourceUpdates.DataStore.WaitForInit(t, data, 3*time.Second)

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

func TestPollingProcessorRecoverableErrors(t *testing.T) {
	for _, statusCode := range []int{400, 408, 429, 500, 503} {
		t.Run(fmt.Sprintf("HTTP %d", statusCode), func(t *testing.T) {
			badResponse := httphelpers.HandlerWithStatus(statusCode)
			testPollingProcessorRecoverableError(t, badResponse, func(errorInfo interfaces.DataSourceErrorInfo) {
				assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, errorInfo.Kind)
				assert.Equal(t, statusCode, errorInfo.StatusCode)
			})
		})

		t.Run("network error", func(t *testing.T) {
			badResponse := httphelpers.PanicHandler(errors.New("sorry")) // this causes the server to drop the connection
			testPollingProcessorRecoverableError(t, badResponse, func(errorInfo interfaces.DataSourceErrorInfo) {
				assert.Equal(t, interfaces.DataSourceErrorKindNetworkError, errorInfo.Kind)
				assert.Contains(t, errorInfo.Message, "EOF") // expected message for this kind of error
			})
		})

		t.Run("malformed data", func(t *testing.T) {
			badResponse := httphelpers.HandlerWithJSONResponse(map[string]interface{}{"flags": 3}, nil)
			testPollingProcessorRecoverableError(t, badResponse, func(errorInfo interfaces.DataSourceErrorInfo) {
				assert.Equal(t, string(interfaces.DataSourceErrorKindInvalidData), string(errorInfo.Kind))
				assert.Contains(t, errorInfo.Message, "cannot unmarshal") // should be a JSON parsing error message
			})
		})
	}
}

func testPollingProcessorRecoverableError(t *testing.T, badResponse http.Handler, verifyError func(interfaces.DataSourceErrorInfo)) {
	handler, requestsCh := httphelpers.RecordingHandler(
		httphelpers.SequentialHandler( // first request fails, second request succeeds
			badResponse,
			ldservices.ServerSidePollingServiceHandler(ldservices.NewServerSDKData()),
		),
	)
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			req := newRequestor(basicClientContext(), nil, ts.URL)
			p := newPollingProcessor(basicClientContext(), dataSourceUpdates, req, time.Millisecond*10)
			defer p.Close()
			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			// wait for first poll
			<-requestsCh

			status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
			verifyError(status.LastError)

			select {
			case <-closeWhenReady:
				require.Fail(t, "should not report ready yet")
			default:
			}

			// wait for second poll
			select {
			case <-requestsCh:
				break
			case <-time.After(time.Second):
				require.Fail(t, "failed to retry")
			}

			waitForReadyWithTimeout(t, closeWhenReady, time.Second)
			_ = dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateValid)
		})
	})
}

func TestPollingProcessorUnrecoverableErrors(t *testing.T) {
	for _, statusCode := range []int{401, 403, 405} {
		t.Run(fmt.Sprintf("HTTP %d", statusCode), func(t *testing.T) {
			badResponse := httphelpers.HandlerWithStatus(statusCode)
			testPollingProcessorUnrecoverableError(t, badResponse, func(errorInfo interfaces.DataSourceErrorInfo) {
				assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, errorInfo.Kind)
				assert.Equal(t, statusCode, errorInfo.StatusCode)
			})
		})
	}
}

func testPollingProcessorUnrecoverableError(t *testing.T, badResponse http.Handler, verifyError func(interfaces.DataSourceErrorInfo)) {
	handler, requestsCh := httphelpers.RecordingHandler(
		httphelpers.SequentialHandler( // first request fails, second request would succeed if it was made
			badResponse,
			ldservices.ServerSidePollingServiceHandler(ldservices.NewServerSDKData()),
		),
	)
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			req := newRequestor(basicClientContext(), nil, ts.URL)
			p := newPollingProcessor(basicClientContext(), dataSourceUpdates, req, time.Millisecond*10)
			defer p.Close()
			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			// wait for first poll
			<-requestsCh

			waitForReadyWithTimeout(t, closeWhenReady, time.Second)

			status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateOff)
			verifyError(status.LastError)
			assert.Len(t, requestsCh, 0)
		})
	})
}

func TestPollingProcessorUsesHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			context := interfaces.NewClientContext(testSdkKey, nil, httpClientFactory, sharedtest.TestLogging())
			req := newRequestor(context, nil, ts.URL)

			p := newPollingProcessor(context, dataSourceUpdates, req, time.Minute*30)
			defer p.Close()
			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			r := <-requestsCh

			assert.Equal(t, "/sdk/latest-all/transformed", r.Request.URL.Path)
		})
	})
}

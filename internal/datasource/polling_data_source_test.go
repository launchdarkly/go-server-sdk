package datasource

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
)

func TestPollingProcessorClosingItShouldNotBlock(t *testing.T) {
	r := mocks.NewPollingRequester()
	defer r.Close()
	r.RequestAllRespCh <- mocks.RequestAllResponse{}

	withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, r, time.Minute)

		p.Close()

		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		th.AssertChannelClosed(t, closeWhenReady, time.Second, "starting a closed processor shouldn't block")
	})
}

func TestPollingProcessorInitialization(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).Build()
	segment := ldbuilders.NewSegmentBuilder("segmentkey").Version(1).Build()

	r := mocks.NewPollingRequester()
	defer r.Close()
	expectedData := sharedtest.NewDataSetBuilder().Flags(flag).Segments(segment)
	resp := mocks.RequestAllResponse{Data: expectedData.Build()}
	r.RequestAllRespCh <- resp

	withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, r, time.Millisecond*10)
		defer p.Close()

		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		if !th.AssertChannelClosed(t, closeWhenReady, time.Second, "Failed to initialize") {
			return
		}

		assert.True(t, p.IsInitialized())

		dataSourceUpdates.DataStore.WaitForInit(t, expectedData.ToServerSDKData(), 2*time.Second)

		for i := 0; i < 2; i++ {
			r.RequestAllRespCh <- resp
			if _, ok, closed := th.TryReceive(r.PollsCh, time.Second); !ok || closed {
				assert.Fail(t, "Expected 2 polls", "but only got %d", i)
				return
			}
		}
	})
}
func TestPollingProcessorRecoverableErrors(t *testing.T) {
	for _, statusCode := range []int{400, 408, 429, 500, 503} {
		t.Run(fmt.Sprintf("HTTP %d", statusCode), func(t *testing.T) {
			testPollingProcessorRecoverableError(
				t,
				httpStatusError{Code: statusCode},
				func(errorInfo interfaces.DataSourceErrorInfo) {
					assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, errorInfo.Kind)
					assert.Equal(t, statusCode, errorInfo.StatusCode)
				},
			)
		})
	}

	t.Run("network error", func(t *testing.T) {
		testPollingProcessorRecoverableError(
			t,
			errors.New("arbitrary error"),
			func(errorInfo interfaces.DataSourceErrorInfo) {
				assert.Equal(t, interfaces.DataSourceErrorKindNetworkError, errorInfo.Kind)
				assert.Equal(t, "arbitrary error", errorInfo.Message)
			},
		)
	})

	t.Run("malformed data", func(t *testing.T) {
		testPollingProcessorRecoverableError(
			t,
			malformedJSONError{innerError: errors.New("sorry")},
			func(errorInfo interfaces.DataSourceErrorInfo) {
				assert.Equal(t, string(interfaces.DataSourceErrorKindInvalidData), string(errorInfo.Kind))
				assert.Contains(t, errorInfo.Message, "sorry")
			},
		)
	})
}

func testPollingProcessorRecoverableError(t *testing.T, err error, verifyError func(interfaces.DataSourceErrorInfo)) {
	req := mocks.NewPollingRequester()
	defer req.Close()

	req.RequestAllRespCh <- mocks.RequestAllResponse{Err: err}

	withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, req, time.Millisecond*10)
		defer p.Close()
		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		// wait for first poll
		<-req.PollsCh

		status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
		verifyError(status.LastError)

		if !th.AssertChannelNotClosed(t, closeWhenReady, 0) {
			t.FailNow()
		}

		req.RequestAllRespCh <- mocks.RequestAllResponse{}

		// wait for second poll
		th.RequireValue(t, req.PollsCh, time.Second, "failed to retry")

		waitForReadyWithTimeout(t, closeWhenReady, time.Second)
		_ = dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateValid)
	})
}

func TestPollingProcessorUnrecoverableErrors(t *testing.T) {
	for _, statusCode := range []int{401, 403, 404, 405} {
		t.Run(fmt.Sprintf("HTTP %d", statusCode), func(t *testing.T) {
			testPollingProcessorUnrecoverableError(
				t,
				httpStatusError{Code: statusCode},
				func(errorInfo interfaces.DataSourceErrorInfo) {
					assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, errorInfo.Kind)
					assert.Equal(t, statusCode, errorInfo.StatusCode)
				},
			)
		})
	}
}

func testPollingProcessorUnrecoverableError(
	t *testing.T,
	err error,
	verifyError func(interfaces.DataSourceErrorInfo),
) {
	req := mocks.NewPollingRequester()
	defer req.Close()

	req.RequestAllRespCh <- mocks.RequestAllResponse{Err: err}
	req.RequestAllRespCh <- mocks.RequestAllResponse{} // we shouldn't get a second request, but just in case

	withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, req, time.Millisecond*10)
		defer p.Close()
		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		// wait for first poll
		<-req.PollsCh

		waitForReadyWithTimeout(t, closeWhenReady, time.Second)

		status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateOff)
		verifyError(status.LastError)
		assert.Len(t, req.PollsCh, 0)
	})
}

func TestPollingProcessorUsesHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
			context := sharedtest.NewTestContext(testSDKKey, &httpConfig, nil)

			p := NewPollingProcessor(context, dataSourceUpdates, PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			})

			defer p.Close()
			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			r := <-requestsCh

			assert.Equal(t, "/sdk/latest-all/transformed", r.Request.URL.Path)
		})
	})
}

func TestPollingProcessorAppendsFilterParameter(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))

	testWithFilters(t, func(t *testing.T, filter filterTest) {
		pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
		httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
			withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
				p := NewPollingProcessor(basicClientContext(), dataSourceUpdates, PollingConfig{
					BaseURI:      ts.URL,
					PollInterval: time.Minute * 30,
					FilterKey:    filter.key,
				})

				defer p.Close()
				closeWhenReady := make(chan struct{})
				p.Start(closeWhenReady)

				r := <-requestsCh

				assert.Equal(t, filter.query, r.Request.URL.RawQuery)
			})
		})
	})
}

package datasource

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/ldservices"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollingProcessorClosingItShouldNotBlock(t *testing.T) {
	r := newMockRequestor()
	defer r.Close()
	r.requestAllRespCh <- mockRequestAllResponse{}

	withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, r, time.Minute)

		p.Close()

		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		select {
		case <-closeWhenReady:
		case <-time.After(time.Second):
			assert.Fail(t, "Start a closed processor shouldn't block")
		}
	})
}

func TestPollingProcessorInitialization(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagkey").Version(1).Build()
	segment := ldbuilders.NewSegmentBuilder("segmentkey").Version(1).Build()

	r := newMockRequestor()
	defer r.Close()
	resp := mockRequestAllResponse{
		data: allData{
			Flags:    map[string]*ldmodel.FeatureFlag{flag.Key: &flag},
			Segments: map[string]*ldmodel.Segment{segment.Key: &segment},
		},
	}
	r.requestAllRespCh <- resp

	expectedData := ldservices.NewServerSDKData().Flags(&flag).Segments(&segment)

	withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, r, time.Millisecond*10)
		defer p.Close()

		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		select {
		case <-closeWhenReady:
		case <-time.After(time.Second):
			assert.Fail(t, "Failed to initialize")
			return
		}

		assert.True(t, p.IsInitialized())

		dataSourceUpdates.DataStore.WaitForInit(t, expectedData, 2*time.Second)

		for i := 0; i < 2; i++ {
			r.requestAllRespCh <- resp
			select {
			case <-r.pollsCh:
			case <-time.After(time.Second):
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
	req := newMockRequestor()
	defer req.Close()

	req.requestAllRespCh <- mockRequestAllResponse{err: err}

	withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, req, time.Millisecond*10)
		defer p.Close()
		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		// wait for first poll
		<-req.pollsCh

		status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
		verifyError(status.LastError)

		select {
		case <-closeWhenReady:
			require.Fail(t, "should not report ready yet")
		default:
		}

		req.requestAllRespCh <- mockRequestAllResponse{}

		// wait for second poll
		select {
		case <-req.pollsCh:
			break
		case <-time.After(time.Second):
			require.Fail(t, "failed to retry")
		}

		waitForReadyWithTimeout(t, closeWhenReady, time.Second)
		_ = dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateValid)
	})
}

func TestPollingProcessorUnrecoverableErrors(t *testing.T) {
	for _, statusCode := range []int{401, 403, 405} {
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
	req := newMockRequestor()
	defer req.Close()

	req.requestAllRespCh <- mockRequestAllResponse{err: err}
	req.requestAllRespCh <- mockRequestAllResponse{} // we shouldn't get a second request, but just in case

	withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
		p := newPollingProcessor(basicClientContext(), dataSourceUpdates, req, time.Millisecond*10)
		defer p.Close()
		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		// wait for first poll
		<-req.pollsCh

		waitForReadyWithTimeout(t, closeWhenReady, time.Second)

		status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateOff)
		verifyError(status.LastError)
		assert.Len(t, req.pollsCh, 0)
	})
}

func TestPollingProcessorUsesHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.FlagOrSegment("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *sharedtest.MockDataSourceUpdates) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			httpConfig := internal.HTTPConfigurationImpl{HTTPClientFactory: httpClientFactory}
			context := sharedtest.NewTestContext(testSDKKey, httpConfig, sharedtest.TestLoggingConfig())

			p := NewPollingProcessor(context, dataSourceUpdates, ts.URL, time.Minute*30)
			defer p.Close()
			closeWhenReady := make(chan struct{})
			p.Start(closeWhenReady)

			r := <-requestsCh

			assert.Equal(t, "/sdk/latest-all/transformed", r.Request.URL.Path)
		})
	})
}

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
		p := newPollingProcessor(sharedtest.BasicClientContext(), dataSourceUpdates, r, time.Minute, 0)

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
		p := newPollingProcessor(sharedtest.BasicClientContext(), dataSourceUpdates, r, time.Millisecond*10, 0)
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
		p := newPollingProcessor(sharedtest.BasicClientContext(), dataSourceUpdates, req, time.Millisecond*10, 0)
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

// 4xx errors (401, 403, 404, 405) engage an extended-regime backoff but keep
// polling indefinitely. The processor transitions to Interrupted (not Off)
// and continues to poll.
func TestPollingProcessorUnexpectedErrorsEngageExtendedRegimeAndKeepRetrying(t *testing.T) {
	for _, statusCode := range []int{401, 403, 404, 405} {
		t.Run(fmt.Sprintf("HTTP %d", statusCode), func(t *testing.T) {
			testPollingProcessorUnexpectedError(
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

func testPollingProcessorUnexpectedError(
	t *testing.T,
	err error,
	verifyError func(interfaces.DataSourceErrorInfo),
) {
	req := mocks.NewPollingRequester()
	defer req.Close()

	// Feed several consecutive failures so we can observe multiple retry attempts.
	for i := 0; i < 5; i++ {
		req.RequestAllRespCh <- mocks.RequestAllResponse{Err: err}
	}

	withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
		// Both PollInterval and ExtendedInitialPollInterval are dialed down so we don't
		// wait the extended-regime 5-minute default between the first failure and
		// observing the second attempt. The extended regime engages internally; here
		// its wait floor is dominated by these two knobs.
		p := newPollingProcessor(
			sharedtest.BasicClientContext(), dataSourceUpdates, req,
			10*time.Millisecond, // PollInterval
			20*time.Millisecond, // ExtendedInitialPollInterval
		)
		defer p.Close()
		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		// Wait for the first poll to fire.
		<-req.PollsCh

		// Initialization must not complete: no permanent stop, no successful poll.
		select {
		case <-closeWhenReady:
			t.Fatal("closeWhenReady should not be closed -- no permanent stops on 4xx")
		case <-time.After(500 * time.Millisecond):
		}

		// Status reports the failure as Interrupted, not Off.
		status := dataSourceUpdates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
		verifyError(status.LastError)

		// Confirm the polling processor kept polling -- at least one additional poll
		// attempt landed at the mock requester.
		select {
		case <-req.PollsCh:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected polling processor to retry after unexpected-classified error")
		}
	})
}

// After two consecutive successful polls, the processor must reset to the normal
// regime: n=0, and subsequent waits equal PollInterval (not the extended initial
// delay). The reset condition is a fixed count of successful polls rather than
// a time threshold.
func TestPollingResetsToNormalAfterTwoConsecutiveSuccesses(t *testing.T) {
	req := mocks.NewPollingRequester()
	defer req.Close()

	// Sequence: fail (engages extended), succeed, succeed (triggers reset), then
	// stay in normal regime. Followed by many placeholder successes so the loop
	// doesn't block.
	req.RequestAllRespCh <- mocks.RequestAllResponse{Err: httpStatusError{Code: 401}}
	for i := 0; i < 10; i++ {
		req.RequestAllRespCh <- mocks.RequestAllResponse{}
	}

	withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
		p := newPollingProcessor(
			sharedtest.BasicClientContext(), dataSourceUpdates, req,
			10*time.Millisecond, // PollInterval
			20*time.Millisecond, // ExtendedInitialPollInterval
		)
		defer p.Close()
		closeWhenReady := make(chan struct{})
		p.Start(closeWhenReady)

		// Poll #1: failure. Extended regime engages internally.
		<-req.PollsCh
		// Poll #2: success. priorPollWasSuccessful flips true; regime not yet reset.
		<-req.PollsCh
		// Poll #3: second consecutive success -- reset should fire (n=0, back to normal).
		<-req.PollsCh
		// Poll #4: normal regime. The wait between polls should be ~PollInterval (10ms), not
		// the extended base. Bounding at 200ms keeps a big margin against goroutine
		// scheduling variance while still catching a regression that would produce
		// a many-second or many-minute wait.
		startPoll4 := time.Now()
		<-req.PollsCh
		elapsed := time.Since(startPoll4)
		assert.Less(t, elapsed, 200*time.Millisecond,
			"after 2 consecutive successes the polling processor should be back at PollInterval cadence")

		// Init completes on the first success.
		waitForReadyWithTimeout(t, closeWhenReady, time.Second)
	})
}

func TestPollingProcessorUsesHTTPClientFactory(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(ldservices.KeyAndVersionItem("my-flag", 2))
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(ts *httptest.Server) {
		withMockDataSourceUpdates(func(dataSourceUpdates *mocks.MockDataSourceUpdates) {
			httpClientFactory := urlAppendingHTTPClientFactory("/transformed")
			httpConfig := subsystems.HTTPConfiguration{CreateHTTPClient: httpClientFactory}
			context := sharedtest.NewTestContext(sharedtest.TestSDKKey, &httpConfig, nil)

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
				p := NewPollingProcessor(sharedtest.BasicClientContext(), dataSourceUpdates, PollingConfig{
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

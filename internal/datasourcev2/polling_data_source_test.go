package datasourcev2

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	"github.com/stretchr/testify/assert"
)

var alwaysTrueFlag = ldbuilders.NewFlagBuilder("always-true-flag").SingleVariation(ldvalue.Bool(true)).Build()

func TestPollingProcessorInitializerCanMakeSuccessfulRequest(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		basis, err := processor.Fetch(ds, context.Background())
		assert.NoError(t, err)
		assert.Len(t, basis.ChangeSet.Changes(), 1)
		assert.Equal(t, basis.ChangeSet.IntentCode(), subsystems.IntentTransferFull)

		r := <-requestsCh
		assert.Equal(t, "/sdk/poll", r.Request.URL.Path)
	})
}

func TestPollingProcessorInitializerAppendsFilterParameter(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				FilterKey:    "filter-value",
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()
		_, err := processor.Fetch(ds, context.Background())
		assert.NoError(t, err)

		r := <-requestsCh
		assert.Equal(t, "/sdk/poll", r.Request.URL.Path)
		assert.Equal(t, "filter-value", r.Request.URL.Query().Get("filter"))
	})
}

func TestPollingProcessorInitializerAppendsBasisParameter(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()
	handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		ds := mocks.NewMockDataSelector(subsystems.NewSelector("test-state", 1))
		_, err := processor.Fetch(ds, context.Background())
		assert.NoError(t, err)

		r := <-requestsCh
		assert.Equal(t, "/sdk/poll", r.Request.URL.Path)
		assert.Equal(t, "test-state", r.Request.URL.Query().Get("basis"))
	})
}

func TestPollingProcessorSynchronizerAppendsFilterParameter(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				FilterKey:    "filter-value",
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		_, err := processor.Fetch(ds, context.Background())
		assert.NoError(t, err)

		r := <-requestsCh
		assert.Equal(t, "/sdk/poll", r.Request.URL.Path)
		assert.Equal(t, "filter-value", r.Request.URL.Query().Get("filter"))
	})
}

func TestPollingProcessorSynchronizerAppendsBasisParameter(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NewSelector("test-state", 1))
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				FilterKey:    "filter-value",
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		_ = processor.Sync(ds)

		r := <-requestsCh
		assert.Equal(t, "/sdk/poll", r.Request.URL.Path)
		assert.Equal(t, "test-state", r.Request.URL.Query().Get("basis"))
	})
}

func TestPollingProcessorSynchronizerPreClosingShouldShutdownImmediately(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	handler, _ := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		processor.Close()

		resultChan := processor.Sync(ds)
		th.AssertChannelClosed(t, resultChan, time.Second, "starting a closed processor should not yield results")
	})
}

func TestPollingProcessorSynchronizerClosingClosesResultChan(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	handler, _ := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)

		resultChan := processor.Sync(ds)
		processor.Close()

		th.AssertChannelClosed(t, resultChan, time.Second, "starting a closed processor should not yield results")
	})
}

func TestPollingProcessorSynchronizerCanMakeSuccessfulRequest(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	handler, _ := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(data))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		resultChan := processor.Sync(ds)
		result := <-resultChan

		assert.Equal(t, result.State, interfaces.DataSourceStateValid)
	})
}

func TestPollingProcessorSynchronizerHandlesInvalidJSON(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())

	handler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(200, nil, []byte("invalid json")))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		resultChan := processor.Sync(ds)
		result := <-resultChan

		assert.Equal(t, result.State, interfaces.DataSourceStateInterrupted)
		assert.Equal(t, result.Error.Kind, interfaces.DataSourceErrorKindInvalidData)
	})
}

func TestPollingProcessorSynchronizerHandlesFallbackToFDv2(t *testing.T) {
	ds := mocks.NewMockDataSelector(subsystems.NoSelector())

	header := http.Header{
		"X-LD-FD-Fallback": []string{"true"},
	}
	handler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(500, header, nil))
	httphelpers.WithServer(handler, func(ts *httptest.Server) {
		processor := NewPollingProcessor(
			sharedtest.BasicClientContext(),
			datasource.PollingConfig{
				BaseURI:      ts.URL,
				PollInterval: time.Minute * 30,
			},
		)
		defer processor.Close()

		resultChan := processor.Sync(ds)
		result := <-resultChan

		assert.Equal(t, result.State, interfaces.DataSourceStateOff)
		assert.Equal(t, result.Error.Kind, interfaces.DataSourceErrorKindErrorResponse)
		assert.True(t, result.RevertToFDv1)
	})
}

func TestPollingProcessorSynchronizerHandlesRecoverableErrors(t *testing.T) {
	for _, statusCode := range []int{400, 408, 429, 500, 503} {
		t.Run(fmt.Sprintf("handles recoverable error %d", statusCode), func(t *testing.T) {
			ds := mocks.NewMockDataSelector(subsystems.NoSelector())
			data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

			handler, requestsCh := httphelpers.RecordingHandler(
				httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(statusCode),
					ldservices.ServerSidePollingV2ServiceHandler(data),
				),
			)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				processor := NewPollingProcessor(
					sharedtest.BasicClientContext(),
					datasource.PollingConfig{
						BaseURI:      ts.URL,
						PollInterval: time.Millisecond,
					},
				)
				defer processor.Close()

				resultChan := processor.Sync(ds)

				<-requestsCh
				result := <-resultChan
				assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)

				<-requestsCh
				result = <-resultChan
				assert.Equal(t, interfaces.DataSourceStateValid, result.State)
				assert.Len(t, result.ChangeSet.Changes(), 1)
			})
		})
	}
}

func TestPollingProcessorSynchronizerHandlesUnrecoverableErrors(t *testing.T) {
	for _, statusCode := range []int{401, 403, 404, 405} {
		t.Run(fmt.Sprintf("handles unrecoverable error %d", statusCode), func(t *testing.T) {
			ds := mocks.NewMockDataSelector(subsystems.NoSelector())

			handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(statusCode))
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				processor := NewPollingProcessor(
					sharedtest.BasicClientContext(),
					datasource.PollingConfig{
						BaseURI:      ts.URL,
						PollInterval: time.Minute * 30,
					},
				)
				defer processor.Close()

				resultChan := processor.Sync(ds)

				<-requestsCh
				result := <-resultChan

				assert.Equal(t, interfaces.DataSourceStateOff, result.State)
				_ = func(errorInfo interfaces.DataSourceErrorInfo) {
					assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, errorInfo.Kind)
					assert.Equal(t, statusCode, errorInfo.StatusCode)
				}
			})
		})
	}
}

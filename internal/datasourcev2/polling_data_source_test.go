package datasourcev2

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	"github.com/stretchr/testify/assert"
)

var (
	alwaysTrueFlag = ldbuilders.NewFlagBuilder("always-true-flag").SingleVariation(ldvalue.Bool(true)).Build()
)

func TestPolllingProcessorAsInitializer(t *testing.T) {
	dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

	t.Run("successful fetch does not change initialization status", func(t *testing.T) {
		handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			processor := NewPollingProcessor(
				sharedtest.BasicClientContext(),
				dd,
				datasource.PollingConfig{
					BaseURI:      ts.URL,
					PollInterval: time.Minute * 30,
				},
			)
			basis, err := processor.Fetch(context.Background())
			assert.NoError(t, err)
			assert.Len(t, basis.Events, 1)

			// Acting as an initializer does not actually affect the initialization
			// status of the processor as a whole.
			assert.False(t, processor.IsInitialized())

			r := <-requestsCh
			assert.Equal(t, "/sdk/latest-all", r.Request.URL.Path)
		})
	})

	t.Run("appends filter parameter", func(t *testing.T) {
		handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			processor := NewPollingProcessor(
				sharedtest.BasicClientContext(),
				dd,
				datasource.PollingConfig{
					BaseURI:      ts.URL,
					FilterKey:    "filter-value",
					PollInterval: time.Minute * 30,
				},
			)
			_, err := processor.Fetch(context.Background())
			assert.NoError(t, err)

			r := <-requestsCh
			assert.Equal(t, "/sdk/latest-all", r.Request.URL.Path)
			assert.Equal(t, "filter-value", r.Request.URL.Query().Get("filter"))
		})
	})
}

func TestPollingProcessorAsSynchronizer(t *testing.T) {
	t.Run("pre-closing should not block close when ready", func(t *testing.T) {
		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

		handler, _ := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			processor := NewPollingProcessor(
				sharedtest.BasicClientContext(),
				dd,
				datasource.PollingConfig{
					BaseURI:      ts.URL,
					PollInterval: time.Minute * 30,
				},
			)
			processor.Close()

			statusChan := processor.Sync()
			th.AssertChannelClosed(t, statusChan, time.Second, "starting a closed processor shouldn't block")
		})
	})

	t.Run("syncing should set initialization", func(t *testing.T) {
		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

		handler, _ := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			processor := NewPollingProcessor(
				sharedtest.BasicClientContext(),
				dd,
				datasource.PollingConfig{
					BaseURI:      ts.URL,
					PollInterval: time.Minute * 30,
				},
			)

			statusChan := processor.Sync()
			result := <-statusChan

			assert.Equal(t, result.State, interfaces.DataSourceStateValid)
			assert.True(t, processor.IsInitialized())
		})
	})

	for _, statusCode := range []int{400, 408, 429, 500, 503} {
		t.Run(fmt.Sprintf("handles recoverable error %d", statusCode), func(t *testing.T) {
			dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
			data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

			handler, requestsCh := httphelpers.RecordingHandler(
				httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(statusCode),
					ldservices.ServerSidePollingServiceHandler(data),
				),
			)
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				processor := NewPollingProcessor(
					sharedtest.BasicClientContext(),
					dd,
					datasource.PollingConfig{
						BaseURI:      ts.URL,
						PollInterval: time.Millisecond,
					},
				)
				statusChan := processor.Sync()

				<-requestsCh
				result := <-statusChan
				assert.Equal(t, interfaces.DataSourceStateInterrupted, result.State)
				assert.False(t, processor.IsInitialized())

				<-requestsCh
				result = <-statusChan
				assert.Equal(t, interfaces.DataSourceStateValid, result.State)
				assert.True(t, processor.IsInitialized())
			})
		})
	}

	for _, statusCode := range []int{401, 403, 404, 405} {
		t.Run(fmt.Sprintf("handles unrecoverable error %d", statusCode), func(t *testing.T) {
			dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))

			handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(statusCode))
			httphelpers.WithServer(handler, func(ts *httptest.Server) {
				processor := NewPollingProcessor(
					sharedtest.BasicClientContext(),
					dd,
					datasource.PollingConfig{
						BaseURI:      ts.URL,
						PollInterval: time.Minute * 30,
					},
				)
				statusChan := processor.Sync()

				<-requestsCh
				result := <-statusChan

				assert.Equal(t, interfaces.DataSourceStateOff, result.State)
				_ = func(errorInfo interfaces.DataSourceErrorInfo) {
					assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, errorInfo.Kind)
					assert.Equal(t, statusCode, errorInfo.StatusCode)
				}

				assert.False(t, processor.IsInitialized())
			})
		})
	}

	t.Run("appends filter parameter", func(t *testing.T) {
		dd := mocks.NewMockDataDestination(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
		data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload()

		handler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
		httphelpers.WithServer(handler, func(ts *httptest.Server) {
			processor := NewPollingProcessor(
				sharedtest.BasicClientContext(),
				dd,
				datasource.PollingConfig{
					BaseURI:      ts.URL,
					FilterKey:    "filter-value",
					PollInterval: time.Minute * 30,
				},
			)
			_, err := processor.Fetch(context.Background())
			assert.NoError(t, err)

			r := <-requestsCh
			assert.Equal(t, "/sdk/latest-all", r.Request.URL.Path)
			assert.Equal(t, "filter-value", r.Request.URL.Query().Get("filter"))
		})
	})
}

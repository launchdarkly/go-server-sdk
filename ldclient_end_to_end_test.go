package ldclient

import (
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/httphelpers"
	"github.com/launchdarkly/go-test-helpers/ldservices"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldhttp"
	shared "gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	initializationFailedErrorMessage = "LaunchDarkly client initialization failed"
	pollingModeWarningMessage        = "You should only disable the streaming API if instructed to do so by LaunchDarkly support"
)

// This file contains smoke tests for a complete SDK instance running against embedded HTTP servers. We have many
// component-level tests elsewhere (including tests of the components' network behavior using an instrumented
// HTTPClient), but the end-to-end tests verify that the client is setting those components up correctly, with a
// configuration that's as close to the default configuration as possible (just changing the service URIs).

func assertNoMoreRequests(t *testing.T, requestsCh <-chan httphelpers.HTTPRequestInfo) {
	assert.Equal(t, 0, len(requestsCh))
}

func TestClientStartsInStreamingMode(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Loggers:    logCapture.Loggers,
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.Nil(t, logCapture.Output[ldlog.Error])
		assert.Nil(t, logCapture.Output[ldlog.Warn])
	})
}

func TestClientFailsToStartInStreamingModeWith401Error(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Loggers:    logCapture.Loggers,
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, initializationFailedErrorMessage, err.Error())

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		expectedError := "Received HTTP error 401 (invalid SDK key) for streaming connection - giving up permanently"
		assert.Equal(t, []string{expectedError}, logCapture.Output[ldlog.Error])
		assert.Equal(t, []string{initializationFailedErrorMessage}, logCapture.Output[ldlog.Warn])
	})
}

func TestClientRetriesConnectionInStreamingModeWithNonFatalError(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
	failThenSucceedHandler := httphelpers.SequentialHandler(httphelpers.HandlerWithStatus(503), streamHandler)
	handler, requestsCh := httphelpers.RecordingHandler(failThenSucceedHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Loggers:    logCapture.Loggers,
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r0 := <-requestsCh
		assert.Equal(t, testSdkKey, r0.Request.Header.Get("Authorization"))
		r1 := <-requestsCh
		assert.Equal(t, testSdkKey, r1.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		expectedError := "Received HTTP error 503 for streaming connection - will retry"
		assert.Equal(t, []string{expectedError}, logCapture.Output[ldlog.Error])
		assert.Nil(t, logCapture.Output[ldlog.Warn])
	})
}

func TestClientStartsInPollingMode(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(pollServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := Config{
			DataSource: ldcomponents.PollingDataSource().BaseURI(pollServer.URL),
			Events:     ldcomponents.NoEvents(),
			Loggers:    logCapture.Loggers,
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.Nil(t, logCapture.Output[ldlog.Error])
		assert.Equal(t, []string{pollingModeWarningMessage}, logCapture.Output[ldlog.Warn])
	})
}

func TestClientFailsToStartInPollingModeWith401Error(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(pollServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := Config{
			DataSource: ldcomponents.PollingDataSource().BaseURI(pollServer.URL),
			Events:     ldcomponents.NoEvents(),
			Loggers:    logCapture.Loggers,
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, initializationFailedErrorMessage, err.Error())

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.NotNil(t, logCapture.Output[ldlog.Error]) // specific error message is long and not important
		assert.Equal(t, []string{pollingModeWarningMessage, initializationFailedErrorMessage}, logCapture.Output[ldlog.Warn])
	})
}

func TestClientSendsEventWithoutDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(eventsServer *httptest.Server) {
		data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
		streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
		httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
			logCapture := shared.NewMockLoggers()

			config := Config{
				DataSource:       ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
				DiagnosticOptOut: true,
				Events:           ldcomponents.SendEvents().BaseURI(eventsServer.URL),
				Loggers:          logCapture.Loggers,
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			client.Identify(testUser)
			client.Flush()

			r := <-eventRequestsCh
			assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
			assert.Equal(t, "/bulk", r.Request.URL.Path)
			assertNoMoreRequests(t, eventRequestsCh)

			var jsonValue ldvalue.Value
			err = json.Unmarshal(r.Body, &jsonValue)
			assert.NoError(t, err)
			assert.Equal(t, ldvalue.String("identify"), jsonValue.GetByIndex(0).GetByKey("kind"))
		})
	})
}

func TestClientSendsDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(eventsServer *httptest.Server) {
		data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
		streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
		httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
			logCapture := shared.NewMockLoggers()

			config := Config{
				DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
				Events:     ldcomponents.SendEvents().BaseURI(eventsServer.URL),
				Loggers:    logCapture.Loggers,
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			r := <-eventRequestsCh
			assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
			assert.Equal(t, "/diagnostic", r.Request.URL.Path)
			var jsonValue ldvalue.Value
			err = json.Unmarshal(r.Body, &jsonValue)
			assert.NoError(t, err)
			assert.Equal(t, ldvalue.String("diagnostic-init"), jsonValue.GetByKey("kind"))
		})
	})
}

func TestClientUsesCustomTLSConfiguration(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)

	httphelpers.WithSelfSignedServer(streamHandler, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
		config := Config{
			DataSource:        ldcomponents.StreamingDataSource().BaseURI(server.URL),
			Events:            ldcomponents.NoEvents(),
			HTTPClientFactory: NewHTTPClientFactory(ldhttp.CACertOption(certData)),
			Loggers:           ldlog.NewDisabledLoggers(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

func TestClientStartupTimesOut(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		streamHandler.ServeHTTP(w, r)
	})

	httphelpers.WithServer(slowHandler, func(streamServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Loggers:    logCapture.Loggers,
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Millisecond*100)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, "timeout encountered waiting for LaunchDarkly client initialization", err.Error())

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		assert.Equal(t, []string{"Timeout encountered waiting for LaunchDarkly client initialization"}, logCapture.Output[ldlog.Warn])
		assert.Nil(t, logCapture.Output[ldlog.Error])
	})
}

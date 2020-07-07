package ldclient

import (
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/ldservices"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	shared "gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	initializationFailedErrorMessage = "LaunchDarkly client initialization failed"
	pollingModeWarningMessage        = "You should only disable the streaming API if instructed to do so by LaunchDarkly support"
)

var (
	alwaysTrueFlag = ldbuilders.NewFlagBuilder("always-true-flag").SingleVariation(ldvalue.Bool(true)).Build()
	testUser       = lduser.NewUser("test-user-key")
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
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
		assert.Len(t, logCapture.GetOutput(ldlog.Warn), 0)
	})
}

func TestClientFailsToStartInStreamingModeWith401Error(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, initializationFailedErrorMessage, err.Error())

		assert.Equal(t, string(interfaces.DataSourceStateOff), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		expectedError := "Error in stream connection (giving up permanently): HTTP error 401 (invalid SDK key)"
		assert.Equal(t, []string{expectedError}, logCapture.GetOutput(ldlog.Error))
		assert.Equal(t, []string{initializationFailedErrorMessage}, logCapture.GetOutput(ldlog.Warn))
	})
}

func TestClientRetriesConnectionInStreamingModeWithNonFatalError(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	failThenSucceedHandler := httphelpers.SequentialHandler(httphelpers.HandlerWithStatus(503), streamHandler)
	handler, requestsCh := httphelpers.RecordingHandler(failThenSucceedHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r0 := <-requestsCh
		assert.Equal(t, testSdkKey, r0.Request.Header.Get("Authorization"))
		r1 := <-requestsCh
		assert.Equal(t, testSdkKey, r1.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		expectedWarning := "Error in stream connection (will retry): HTTP error 503"
		assert.Equal(t, []string{expectedWarning}, logCapture.GetOutput(ldlog.Warn))
		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
	})
}

func TestClientStartsInPollingMode(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(pollServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource: ldcomponents.PollingDataSource().BaseURI(pollServer.URL),
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
		assert.Equal(t, []string{pollingModeWarningMessage}, logCapture.GetOutput(ldlog.Warn))
	})
}

func TestClientFailsToStartInPollingModeWith401Error(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(pollServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource: ldcomponents.PollingDataSource().BaseURI(pollServer.URL),
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, initializationFailedErrorMessage, err.Error())

		assert.Equal(t, string(interfaces.DataSourceStateOff), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		expectedError := "Error on polling request (giving up permanently): HTTP error 401 (invalid SDK key)"
		assert.Equal(t, []string{expectedError}, logCapture.GetOutput(ldlog.Error))
		assert.Equal(t, []string{pollingModeWarningMessage, initializationFailedErrorMessage}, logCapture.GetOutput(ldlog.Warn))
	})
}

func TestClientSendsEventWithoutDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(eventsServer *httptest.Server) {
		data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
		streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
		httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				DataSource:       ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
				DiagnosticOptOut: true,
				Events:           ldcomponents.SendEvents().BaseURI(eventsServer.URL),
				Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
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
		streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
		httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
			config := Config{
				DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
				Events:     ldcomponents.SendEvents().BaseURI(eventsServer.URL),
				Logging:    shared.TestLogging(),
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
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())

	httphelpers.WithSelfSignedServer(streamHandler, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(server.URL),
			Events:     ldcomponents.NoEvents(),
			HTTP:       ldcomponents.HTTPConfiguration().CACert(certData),
			Logging:    shared.TestLogging(),
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
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		streamHandler.ServeHTTP(w, r)
	})

	httphelpers.WithServer(slowHandler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource: ldcomponents.StreamingDataSource().BaseURI(streamServer.URL),
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Millisecond*100)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, "timeout encountered waiting for LaunchDarkly client initialization", err.Error())

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		assert.Equal(t, []string{"Timeout encountered waiting for LaunchDarkly client initialization"}, logCapture.GetOutput(ldlog.Warn))
		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
	})
}

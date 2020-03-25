package ldclient

import (
	"crypto/x509"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/httphelpers"
	"github.com/launchdarkly/go-test-helpers/ldservices"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v1/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v4/ldhttp"
	"gopkg.in/launchdarkly/go-server-sdk.v4/ldlog"
	shared "gopkg.in/launchdarkly/go-server-sdk.v4/shared_test"
)

// This file contains smoke tests for a complete SDK instance running against embedded HTTP servers. We have many
// component-level tests elsewhere (including tests of the components' network behavior using an instrumented
// HTTPClient), but the end-to-end tests verify that the client is setting those components up correctly, with a
// configuration that's as close to the default configuration as possible (just changing the service URIs).

const testSdkKey = "sdk-key"

var testUser = NewUser("test-user-key")

var alwaysTrueFlag = FeatureFlag{
	Key:          "always-true-flag",
	Version:      1,
	On:           false,
	OffVariation: intPtr(0),
	Variations:   []interface{}{true},
}

func assertNoMoreRequests(t *testing.T, requestsCh <-chan httphelpers.HTTPRequestInfo) {
	assert.Equal(t, 0, len(requestsCh))
}

func TestClientStartsInStreamingMode(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := DefaultConfig
		config.StreamUri = streamServer.URL
		config.SendEvents = false
		config.Loggers = logCapture.Loggers

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
	streamServer := httptest.NewServer(handler)
	defer streamServer.Close()

	logCapture := shared.NewMockLoggers()

	config := DefaultConfig
	config.StreamUri = streamServer.URL
	config.SendEvents = false
	config.Loggers = logCapture.Loggers

	client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.Error(t, err)
	require.NotNil(t, client)
	defer client.Close()

	value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
	assert.False(t, value)

	r := <-requestsCh
	assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
	assertNoMoreRequests(t, requestsCh)
}

func TestClientRetriesConnectionInStreamingModeWithNonFatalError(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
	failThenSucceedHandler := httphelpers.SequentialHandler(httphelpers.HandlerWithStatus(503), streamHandler)
	handler, requestsCh := httphelpers.RecordingHandler(failThenSucceedHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := DefaultConfig
		config.StreamUri = streamServer.URL
		config.SendEvents = false
		config.Loggers = logCapture.Loggers

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

		assert.Equal(t, 1, len(logCapture.Output[ldlog.Error]))
		assert.Equal(t, 1, len(logCapture.Output[ldlog.Warn]))
	})
}

func TestClientStartsInPollingMode(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(pollServer *httptest.Server) {
		logCapture := shared.NewMockLoggers()

		config := DefaultConfig
		config.Stream = false
		config.BaseUri = pollServer.URL
		config.SendEvents = false
		config.Loggers = logCapture.Loggers

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.Nil(t, logCapture.Output[ldlog.Error])
		assert.Equal(t, []string{
			"You should only disable the streaming API if instructed to do so by LaunchDarkly support",
		}, logCapture.Output[ldlog.Warn])
	})
}

func TestClientFailsToStartInPollingModeWith401Error(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	pollServer := httptest.NewServer(handler)
	defer pollServer.Close()

	logCapture := shared.NewMockLoggers()

	config := DefaultConfig
	config.Stream = false
	config.BaseUri = pollServer.URL
	config.SendEvents = false
	config.Loggers = logCapture.Loggers

	client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.Error(t, err)
	require.NotNil(t, client)
	defer client.Close()

	value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
	assert.False(t, value)

	r := <-requestsCh
	assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
	assertNoMoreRequests(t, requestsCh)
}

func TestClientSendsEventWithoutDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(eventsServer *httptest.Server) {
		data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
		streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data, nil)
		httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
			logCapture := shared.NewMockLoggers()

			config := DefaultConfig
			config.EventsUri = eventsServer.URL
			config.StreamUri = streamServer.URL
			config.DiagnosticOptOut = true
			config.Loggers = logCapture.Loggers

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

			config := DefaultConfig
			config.EventsUri = eventsServer.URL
			config.StreamUri = streamServer.URL
			config.Loggers = logCapture.Loggers

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
		config := DefaultConfig
		config.HTTPClientFactory = NewHTTPClientFactory(ldhttp.CACertOption(certData))
		config.StreamUri = server.URL
		config.SendEvents = false

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

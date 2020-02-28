package ldclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func makeFlagsData(flags ...*FeatureFlag) *shared.SDKData {
	flagsMap := make(map[string]*FeatureFlag, len(flags))
	for _, f := range flags {
		flagsMap[f.Key] = f
	}
	bytes, _ := json.Marshal(flagsMap)
	return &shared.SDKData{FlagsData: bytes}
}

func assertNoMoreRequests(t *testing.T, requestsCh <-chan shared.HTTPRequestInfo) {
	assert.Equal(t, 0, len(requestsCh))
}

func TestClientStartsInStreamingMode(t *testing.T) {
	streamHandler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewStreamingServiceHandler(makeFlagsData(&alwaysTrueFlag), nil))
	streamServer := httptest.NewServer(streamHandler)
	defer streamServer.Close()

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
}

func TestClientFailsToStartInStreamingModeWith401Error(t *testing.T) {
	handler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewHTTPHandlerReturningStatus(401))
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
	streamHandler := shared.NewStreamingServiceHandler(makeFlagsData(&alwaysTrueFlag), nil)
	requestCount := 0
	handler, requestsCh := shared.NewRecordingHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.WriteHeader(503)
		} else {
			streamHandler.ServeHTTP(w, r)
		}
	}))
	streamServer := httptest.NewServer(handler)
	defer streamServer.Close()

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
}

func TestClientStartsInPollingMode(t *testing.T) {
	pollHandler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewPollingServiceHandler(*makeFlagsData(&alwaysTrueFlag)))
	pollServer := httptest.NewServer(pollHandler)
	defer pollServer.Close()

	logCapture := shared.NewMockLoggers()

	config := DefaultConfig
	config.Stream = false
	config.BaseUri = pollServer.URL
	config.SendEvents = false
	config.Loggers = logCapture.Loggers

	client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
	fmt.Println(logCapture.AllOutput)
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
}

func TestClientFailsToStartInPollingModeWith401Error(t *testing.T) {
	handler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewHTTPHandlerReturningStatus(401))
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
	eventsHandler, eventRequestsCh := shared.NewRecordingHTTPHandler(shared.NewEventsServiceHandler())
	eventsServer := httptest.NewServer(eventsHandler)
	defer eventsServer.Close()

	streamHandler := shared.NewStreamingServiceHandler(makeFlagsData(&alwaysTrueFlag), nil)
	streamServer := httptest.NewServer(streamHandler)
	defer streamServer.Close()

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
}

func TestClientSendsDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := shared.NewRecordingHTTPHandler(shared.NewEventsServiceHandler())
	eventsServer := httptest.NewServer(eventsHandler)
	defer eventsServer.Close()

	streamHandler := shared.NewStreamingServiceHandler(makeFlagsData(&alwaysTrueFlag), nil)
	streamServer := httptest.NewServer(streamHandler)
	defer streamServer.Close()

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
}

func TestClientUsesCustomTLSConfiguration(t *testing.T) {
	streamHandler := shared.NewStreamingServiceHandler(makeFlagsData(&alwaysTrueFlag), nil)

	shared.WithTempFile(func(certFile string) {
		shared.WithTempFile(func(keyFile string) {
			err := shared.MakeSelfSignedCert(certFile, keyFile)
			require.NoError(t, err)

			server, err := shared.MakeServerWithCert(certFile, keyFile, streamHandler)
			require.NoError(t, err)
			defer server.Close()

			config := DefaultConfig
			config.HTTPClientFactory = NewHTTPClientFactory(ldhttp.CACertFileOption(certFile))
			config.StreamUri = server.URL
			config.SendEvents = false

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
			assert.True(t, value)
		})
	})
}

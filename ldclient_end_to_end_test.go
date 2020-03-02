package ldclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldhttp"
	shared "gopkg.in/launchdarkly/go-server-sdk.v5/shared_test"
)

// This file contains smoke tests for a complete SDK instance running against embedded HTTP servers. We have many
// component-level tests elsewhere (including tests of the components' network behavior using an instrumented
// HTTPClient), but the end-to-end tests verify that the client is setting those components up correctly, with a
// configuration that's as close to the default configuration as possible (just changing the service URIs).

func assertNoMoreRequests(t *testing.T, requestsCh <-chan shared.HTTPRequestInfo) {
	assert.Equal(t, 0, len(requestsCh))
}

func TestClientStartsInStreamingMode(t *testing.T) {
	streamHandler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewStreamingServiceHandler(makeFlagsData(&alwaysTrueFlag), nil))
	streamServer := httptest.NewServer(streamHandler)
	defer streamServer.Close()

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
}

func TestClientFailsToStartInStreamingModeWith401Error(t *testing.T) {
	handler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewHTTPHandlerReturningStatus(401))
	streamServer := httptest.NewServer(handler)
	defer streamServer.Close()

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

	assert.Equal(t, 1, len(logCapture.Output[ldlog.Error]))
	assert.Equal(t, 1, len(logCapture.Output[ldlog.Warn]))
}

func TestClientStartsInPollingMode(t *testing.T) {
	pollHandler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewPollingServiceHandler(*makeFlagsData(&alwaysTrueFlag)))
	pollServer := httptest.NewServer(pollHandler)
	defer pollServer.Close()

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
	assert.Equal(t, []string{
		"You should only disable the streaming API if instructed to do so by LaunchDarkly support",
	}, logCapture.Output[ldlog.Warn])
}

func TestClientFailsToStartInPollingModeWith401Error(t *testing.T) {
	handler, requestsCh := shared.NewRecordingHTTPHandler(shared.NewHTTPHandlerReturningStatus(401))
	pollServer := httptest.NewServer(handler)
	defer pollServer.Close()

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
}

func TestClientSendsDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := shared.NewRecordingHTTPHandler(shared.NewEventsServiceHandler())
	eventsServer := httptest.NewServer(eventsHandler)
	defer eventsServer.Close()

	streamHandler := shared.NewStreamingServiceHandler(makeFlagsData(&alwaysTrueFlag), nil)
	streamServer := httptest.NewServer(streamHandler)
	defer streamServer.Close()

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

			config := Config{
				DataSource:        ldcomponents.StreamingDataSource().BaseURI(server.URL),
				Events:            ldcomponents.NoEvents(),
				HTTPClientFactory: NewHTTPClientFactory(ldhttp.CACertFileOption(certFile)),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
			assert.True(t, value)
		})
	})
}

package ldclient

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const newTestSdkKey = "new-test-sdk-key-123456"

func TestSetSDKKeyStreamReconnectUsesNewKey(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, streamControl := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		config := Config{
			DataSource:       &compressedStreamingBuilder{initialReconnectDelay: 10 * time.Millisecond},
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()
		first := th.RequireValue(t, requestsCh, time.Second, "timed out waiting for first stream request")
		assert.Equal(t, testSdkKey, first.Request.Header.Get("Authorization"))

		// Change the key, then end the open stream so that the client reconnects.
		require.NoError(t, client.SetSDKKey(newTestSdkKey))
		streamControl.EndAll()

		// The reconnection should use the new key.
		second := th.RequireValue(t, requestsCh, time.Second*5, "timed out waiting for stream reconnection")
		assert.Equal(t, newTestSdkKey, second.Request.Header.Get("Authorization"))
	})
}

func TestSetSDKKeyFDv2StreamReconnectUsesNewKey(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)
	streamHandler, streamControl := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		streaming := ldcomponents.StreamingDataSourceV2().
			BaseURI(streamServer.URL).
			InitialReconnectDelay(10 * time.Millisecond)
		config := Config{
			DataSystem: ldcomponents.DataSystem().Custom().Synchronizers(streaming),
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()
		first := th.RequireValue(t, requestsCh, time.Second, "timed out waiting for first stream request")
		assert.Equal(t, testSdkKey, first.Request.Header.Get("Authorization"))

		// Change the key, then end the open stream so that the client reconnects.
		require.NoError(t, client.SetSDKKey(newTestSdkKey))
		streamControl.EndAll()

		// The reconnection should use the new key.
		second := th.RequireValue(t, requestsCh, time.Second*5, "timed out waiting for stream reconnection")
		assert.Equal(t, newTestSdkKey, second.Request.Header.Get("Authorization"))
	})
}

func TestSetSDKKeyEventsUseNewKey(t *testing.T) {
	eventsHandler, eventRequestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(eventsServer *httptest.Server) {
		config := Config{
			DataSource:       ldcomponents.ExternalUpdatesOnly(),
			DiagnosticOptOut: true,
			Logging:          ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
			ServiceEndpoints: interfaces.ServiceEndpoints{Events: eventsServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		// Change the key, then send an event.
		require.NoError(t, client.SetSDKKey(newTestSdkKey))
		client.Identify(testUser)
		client.Flush()

		// The event delivery should use the new key.
		r := th.RequireValue(t, eventRequestsCh, time.Second*5, "timed out waiting for event delivery")
		assert.Equal(t, "/bulk", r.Request.URL.Path)
		assert.Equal(t, newTestSdkKey, r.Request.Header.Get("Authorization"))
	})
}

func TestSetSDKKeyChangesDiagnosticIDSuffix(t *testing.T) {
	config := Config{
		DataSource: ldcomponents.ExternalUpdatesOnly(),
		Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
	}
	client, err := MakeCustomClient(testSdkKey, config, 0)
	require.NoError(t, err)
	defer client.Close()
	originalID := client.diagnosticsManager.CreateStatsEventAndReset(0, 0, 0).GetByKey("id")

	// Change the key.
	require.NoError(t, client.SetSDKKey(newTestSdkKey))

	// The diagnostic ID should carry the new suffix and keep its unique identifier.
	id := client.diagnosticsManager.CreateStatsEventAndReset(0, 0, 0).GetByKey("id")
	assert.Equal(t, ldvalue.String("123456"), id.GetByKey("sdkKeySuffix"))
	assert.Equal(t, originalID.GetByKey("diagnosticId"), id.GetByKey("diagnosticId"))
}

func TestSetSDKKeyChangesSecureModeHash(t *testing.T) {
	client, _ := MakeCustomClient(testSdkKey, Config{Offline: true}, 0)
	defer client.Close()
	expected, _ := MakeCustomClient(newTestSdkKey, Config{Offline: true}, 0)
	defer expected.Close()

	// Change the key.
	require.NoError(t, client.SetSDKKey(newTestSdkKey))

	// The hash should match the hash of a client created with the new key.
	assert.Equal(t, expected.SecureModeHash(testUser), client.SecureModeHash(testUser))
}

func TestSetSDKKeyRejectsInvalidKey(t *testing.T) {
	client, _ := MakeCustomClient(testSdkKey, Config{Offline: true}, 0)
	defer client.Close()
	hashBefore := client.SecureModeHash(testUser)

	// Try to set a key that is not a valid HTTP header value.
	err := client.SetSDKKey("bad-key\n")

	// The call should fail, and the client should keep its current key.
	assert.Equal(t, errSDKKeyInvalidCharacters, err)
	assert.Equal(t, hashBefore, client.SecureModeHash(testUser))
}

package ldclient

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This tests the sequential two-phase nature of the Flag Delivery V2 protocol. First, a polling initializer
// attempts to grab a payload, but the mock handler will return a 500. The initializer will fail and
// the primary synchronizer (streaming mode) will then make a streaming request. This succeeds, returning a payload
// containing a true flag.
func TestFDV2DefaultIsTwoPhaseInit(t *testing.T) {

	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	// The polling initializer will fail since we return a 500.
	pollRecordingHandler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(500))

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	// The streaming synchronizer will receive the FDv2 protocol messages, including the true flag.
	streamHandler, streamSender := ldservices.ServerSideStreamingV2ServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	streamRecordingHandler, _ := httphelpers.RecordingHandler(streamHandler)

	// Use a sequential handler so that the first request is serviced by the polling handler, and the second
	// by the streaming.
	handler := httphelpers.SequentialHandler(pollRecordingHandler, streamRecordingHandler)

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Default(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

	})
}

func TestFDV2CanFallBackToV1(t *testing.T) {
	dataV1 := ldservices.NewServerSDKData().Flags(alwaysFalseFlag)
	dataV2 := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	header := http.Header{
		"X-LD-FD-Fallback": []string{"true"},
	}
	// The polling initializer will fail since we return a 500.
	pollV1SyncRecordingHandler, pollV1SyncReqCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(dataV1))
	pollV2InitRecordingHandler, pollV2SyncReqCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingV2ServiceHandler(dataV2))

	streamHandler, streamV2SyncReqCh := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(500, header, nil))

	handler := httphelpers.SequentialHandler(pollV2InitRecordingHandler, streamHandler, pollV1SyncRecordingHandler)

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Default(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)

		<-pollV2SyncReqCh
		<-streamV2SyncReqCh
		<-pollV1SyncReqCh

		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		assertNoMoreRequests(t, pollV2SyncReqCh)
		assertNoMoreRequests(t, streamV2SyncReqCh)

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, true)
		assert.False(t, value)

	})
}

func TestFDV2StreamingSynchronizer(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingV2ServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.DumpIfTestFailed(t)

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem: ldcomponents.DataSystem().WithEndpoints(
				ldcomponents.Endpoints{Streaming: streamServer.URL},
			).Streaming(),
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

func TestFDV2ShutdownDownIfBothSynchronizersFail(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		dataSystemBuilder := ldcomponents.DataSystem().Custom().Synchronizers(
			ldcomponents.StreamingDataSourceV2().BaseURI(server.URL),
			ldcomponents.PollingDataSourceV2().BaseURI(server.URL),
		)

		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: dataSystemBuilder,
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, initializationFailedErrorMessage, err.Error())

		assert.Equal(t, string(interfaces.DataSourceStateOff), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		// Streaming request
		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))

		// Polling request
		r = <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))

		// Ensure no further requests
		assertNoMoreRequests(t, requestsCh)

		expectedStreamError := "Error in stream connection (giving up permanently): HTTP error 401 (invalid SDK key)"
		expectedPollError := "Error on polling request (giving up permanently): HTTP error 401 (invalid SDK key)"
		assert.Equal(t, []string{expectedStreamError, expectedPollError}, logCapture.GetOutput(ldlog.Error))
		assert.Equal(t, []string{pollingModeWarningMessage, initializationFailedErrorMessage}, logCapture.GetOutput(ldlog.Warn))
	})
}

func TestFDV2StreamingSynchronizeReconnectsWithNonFatalError(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingV2ServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	failThenSucceedHandler := httphelpers.SequentialHandler(httphelpers.HandlerWithStatus(503), streamHandler)
	handler, requestsCh := httphelpers.RecordingHandler(failThenSucceedHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem: ldcomponents.DataSystem().WithEndpoints(
				ldcomponents.Endpoints{Streaming: streamServer.URL}).Streaming(),
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

func TestFDV2PollingSynchronizerFailsToStartWith401Error(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(pollServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:  ldcomponents.NoEvents(),
			Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().
				WithEndpoints(ldcomponents.Endpoints{Polling: pollServer.URL}).Polling(),
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

func TestFDV2StreamingSynchronizerUsesCustomTLSConfiguration(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingV2ServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	httphelpers.WithSelfSignedServer(streamHandler, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {

		config := Config{
			Events:  ldcomponents.NoEvents(),
			HTTP:    ldcomponents.HTTPConfiguration().CACert(certData),
			Logging: ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
			DataSystem: ldcomponents.DataSystem().WithEndpoints(
				ldcomponents.Endpoints{Streaming: server.URL}).Streaming(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*50000)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

func TestFDV2StreamingSynchronizerTimesOut(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingV2ServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		streamHandler.ServeHTTP(w, r)
	})

	httphelpers.WithServer(slowHandler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem: ldcomponents.DataSystem().WithEndpoints(
				ldcomponents.Endpoints{Streaming: streamServer.URL}).Streaming(),
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

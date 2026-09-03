package ldclient

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldfiledatav2"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	testHelpers "github.com/launchdarkly/go-test-helpers/v3"
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
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)

	// The streaming synchronizer will receive the FDv2 protocol messages, including the true flag.
	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

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

		reached := client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second*5)
		require.True(t, reached, "timed out waiting for data source to reach VALID state")

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

		reached := client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second*5)
		require.True(t, reached, "timed out waiting for data source to reach VALID state")

		assertNoMoreRequests(t, pollV2SyncReqCh)
		assertNoMoreRequests(t, streamV2SyncReqCh)

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, true)
		assert.False(t, value)
	})
}

// When an initializer requests FDv1 fallback but no FDv1 fallback is configured, the data source
// status must transition to Off rather than staying stuck at Initializing. This mirrors the
// synchronizer-triggered path when fdv1FallbackBuilder is nil.
func TestFDV2InitializerFallbackWithoutFDv1FallbackTransitionsToOff(t *testing.T) {
	header := http.Header{
		"X-LD-FD-Fallback": []string{"true"},
	}
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(500, header, nil))

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		// Custom data system: a polling initializer, no synchronizers, no FDv1 fallback.
		config := Config{
			Events:  ldcomponents.NoEvents(),
			Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().Custom().Initializers(
				ldcomponents.PollingDataSourceV2().BaseURI(server.URL).AsInitializer(),
			),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		<-requestsCh

		// With no FDv1 fallback configured, an initializer-triggered fallback must transition the
		// status to Off — if it stays at Initializing, MakeCustomClient treats it as an init
		// failure and we see initializationFailedErrorMessage here. Either way the status field
		// should end up Off, so assert that directly.
		status := client.GetDataSourceStatusProvider().GetStatus()
		assert.Equal(t,
			interfaces.DataSourceStateOff,
			status.State,
			"status should transition to Off when initializer fallback requested but no FDv1 fallback configured")
		// The underlying initializer error must be preserved on the Off status so programmatic
		// monitors can see why the data source shut down, not just that it did.
		assert.NotEqual(t, interfaces.DataSourceErrorInfo{}, status.LastError,
			"LastError should carry the initializer error that accompanied the fallback signal")
		assert.Equal(t, initializationFailedErrorMessage, err.Error())

		assert.Contains(t, logCapture.GetOutput(ldlog.Warn),
			"Initializer requested FDv1 fallback but none configured")
	})
}

// When the streaming synchronizer receives a 200 response that carries both a valid SSE payload
// AND the x-ld-fd-fallback header, the SDK should apply the payload and then fall back to FDv1.
// Without this behavior, the stream stays open against the FDv2 endpoint indefinitely.
func TestFDV2CanFallBackToV1FromStreamingSuccess(t *testing.T) {
	dataV1 := ldservices.NewServerSDKData().Flags(alwaysFalseFlag)
	dataV2 := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(dataV2.ToPutObjects()).
		WithTransferred("state", 1)

	streamV2Handler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)
	streamV2WithFallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-LD-FD-Fallback", "true")
		streamV2Handler.ServeHTTP(w, r)
	})

	// Init phase: FDv2 poll returns 500. Sync phase: FDv2 stream returns valid SSE + fallback
	// header. FDv1 fallback phase: FDv1 poll returns the V1 data (always-false flag).
	pollV2InitHandler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(500))
	streamRecordingHandler, streamV2ReqCh := httphelpers.RecordingHandler(streamV2WithFallback)
	pollV1SyncHandler, pollV1SyncReqCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(dataV1))

	handler := httphelpers.SequentialHandler(pollV2InitHandler, streamRecordingHandler, pollV1SyncHandler)

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Default(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		<-streamV2ReqCh
		<-pollV1SyncReqCh
		require.NoError(t, err)
		defer client.Close()

		reached := client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second*5)
		require.True(t, reached, "timed out waiting for data source to reach VALID state")

		// Status becomes Valid as soon as the FDv2 stream applies its payload (with FallbackToFDv1
		// riding along on the same result), which happens before FDv1 has fetched its own data.
		// Poll until the flag value reflects FDv1 data to verify the handoff completed.
		assert.Eventually(t, func() bool {
			value, _ := client.BoolVariation(alwaysFalseFlag.Key, testUser, true)
			return value == false
		}, time.Second*2, time.Millisecond*10, "expected FDv1 data (value=false) to replace FDv2 data")
	})
}

// When the polling initializer receives x-ld-fd-fallback from the server, the SDK should skip any
// remaining FDv2 synchronizers and switch to the FDv1 polling synchronizer directly — without ever
// attempting the FDv2 streaming synchronizer.
func TestFDV2CanFallBackToV1FromInitializer(t *testing.T) {
	dataV1 := ldservices.NewServerSDKData().Flags(alwaysFalseFlag)

	header := http.Header{
		"X-LD-FD-Fallback": []string{"true"},
	}

	// FDv2 polling initializer: returns 500 + fallback header. Must trigger fallback to FDv1 before
	// the FDv2 streaming synchronizer is ever dialed.
	pollV2InitRecordingHandler, pollV2InitReqCh := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(500, header, nil))
	// FDv1 polling synchronizer: returns valid FDv1 data.
	pollV1SyncRecordingHandler, pollV1SyncReqCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(dataV1))
	// FDv2 streaming synchronizer: should never be hit. If it is, the test will surface it via
	// streamV2SyncReqCh.
	streamHandler, streamV2SyncReqCh := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(500, header, nil))

	handler := httphelpers.SequentialHandler(pollV2InitRecordingHandler, pollV1SyncRecordingHandler, streamHandler)

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Default(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)

		<-pollV2InitReqCh
		<-pollV1SyncReqCh

		require.NoError(t, err)
		defer client.Close()

		reached := client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second*5)
		require.True(t, reached, "timed out waiting for data source to reach VALID state")

		assertNoMoreRequests(t, streamV2SyncReqCh)

		value, _ := client.BoolVariation(alwaysFalseFlag.Key, testUser, true)
		assert.False(t, value)
	})
}

func TestFDV2StreamingSynchronizer(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)

	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

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

		// A successful start must leave the data source in the valid state; the status update
		// must not lag behind the readiness signal.
		assert.Equal(t,
			string(interfaces.DataSourceStateValid),
			string(client.GetDataSourceStatusProvider().GetStatus().State))

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
		assert.Equal(t, []string{
			"Permanently removing synchronizer at index 0",
			"Permanently removing synchronizer at index 0",
			"No more synchronizers available",
			initializationFailedErrorMessage,
		}, logCapture.GetOutput(ldlog.Warn))
	})
}

func TestFDV2StreamingSynchronizeReconnectsWithNonFatalError(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)

	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

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

		reached := client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second*5)
		require.True(t, reached, "timed out waiting for data source to reach VALID state")

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
		assert.Equal(t, []string{
			"Permanently removing synchronizer at index 0",
			"No more synchronizers available",
			initializationFailedErrorMessage,
		}, logCapture.GetOutput(ldlog.Warn))
	})
}

func TestFDV2StreamingSynchronizerUsesCustomTLSConfiguration(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)

	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

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
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)

	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

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

// A file initializer returns data without a selector, and the streaming synchronizer later replaces
// that data. This test checks the refresh. TestFDV2InitializerDataWithoutSelectorCompletesInitialization
// checks that the initializer data alone completes initialization.
func TestFDV2SynchronizerRefreshesDataAfterInitializerCompletesInit(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)

	streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)

	testHelpers.WithTempFileData([]byte(`{"flags": {"`+alwaysFalseFlag.Key+`": {"on": false}}, "segments": {}}`), func(filename string) {
		httphelpers.WithServer(handler, func(server *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
				DataSystem: ldcomponents.DataSystem().Custom().
					Initializers(
						ldfiledatav2.DataSource().FilePaths(filename).AsInitializer(),
					).
					Synchronizers(
						ldcomponents.StreamingDataSourceV2().BaseURI(server.URL),
					),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)

			<-requestsCh

			require.NoError(t, err)
			defer client.Close()

			assert.True(t, client.Initialized())

			// The status is already Valid from the initializer data, so wait for the synchronizer's
			// data by observing the evaluation result.
			assert.Eventually(t, func() bool {
				value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
				return value
			}, time.Second*5, time.Millisecond*20, "synchronizer did not replace the initializer data")
		})
	})
}

// Initialization must complete when all initializers have run and any of them returned data,
// even though that data has no selector. The synchronizer never delivers data, so a successful
// (non-timeout) client start proves that the initializer data completed initialization.
func TestFDV2InitializerDataWithoutSelectorCompletesInitialization(t *testing.T) {
	testHelpers.WithTempFileData(fileDataWithOneFlag, func(filename string) {
		httphelpers.WithServer(hangingStreamHandler, func(server *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				Events:  ldcomponents.NoEvents(),
				Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
				DataSystem: ldcomponents.DataSystem().Custom().
					Initializers(
						ldfiledatav2.DataSource().FilePaths(filename).AsInitializer(),
					).
					Synchronizers(
						ldcomponents.StreamingDataSourceV2().BaseURI(server.URL),
					),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			assert.True(t, client.Initialized())
			assert.Equal(t, interfaces.DataSourceStateValid, client.GetDataSourceStatusProvider().GetStatus().State)

			value, _ := client.BoolVariation("flag-from-file", testUser, false)
			assert.True(t, value)
		})
	})
}

// A persistent store is a data store, not a data source. Data that a previous SDK instance
// persisted must not satisfy the initialization success signal when every configured data
// source fails. The client still reports Initialized and serves the persisted data.
func TestFDV2PopulatedPersistentStoreDoesNotSatisfyInitSuccess(t *testing.T) {
	flag := alwaysTrueFlag
	persistentStore := mocks.NewMockPersistentDataStore()
	require.NoError(t, persistentStore.Init([]ldstoretypes.SerializedCollection{
		{
			Kind: datakinds.Features,
			Items: []ldstoretypes.KeyedSerializedItemDescriptor{
				{
					Key: flag.Key,
					Item: ldstoretypes.SerializedItemDescriptor{
						Version: flag.Version,
						SerializedItem: datakinds.Features.Serialize(
							ldstoretypes.ItemDescriptor{Version: flag.Version, Item: &flag}),
					},
				},
			},
		},
	}))

	// The initializer fails because its file does not exist; the streaming synchronizer fails
	// permanently with a 401.
	handler := httphelpers.HandlerWithStatus(401)

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:  ldcomponents.NoEvents(),
			Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().Custom().
				Initializers(
					ldfiledatav2.DataSource().FilePaths("does-not-exist.json").AsInitializer(),
				).
				Synchronizers(
					ldcomponents.StreamingDataSourceV2().BaseURI(server.URL),
				).
				DataStore(ldcomponents.PersistentDataStore(
					mocks.SingleComponentConfigurer[subsystems.PersistentDataStore]{Instance: persistentStore},
				), subsystems.DataStoreModeReadWrite),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()
		assert.Equal(t, initializationFailedErrorMessage, err.Error())

		// The persisted data is still available: the client reports Initialized and evaluates
		// with the stored flag.
		assert.True(t, client.Initialized())
		value, _ := client.BoolVariation(flag.Key, testUser, false)
		assert.True(t, value)
	})
}

// The first initializer returns data without a selector, so the loop continues to the next
// initializer. The next initializer fails. Initialization must still complete with the data
// from the first initializer.
func TestFDV2InitializerDataIsRetainedWhenLaterInitializerFails(t *testing.T) {
	testHelpers.WithTempFileData(fileDataWithOneFlag, func(filename string) {
		httphelpers.WithServer(hangingStreamHandler, func(server *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				Events:  ldcomponents.NoEvents(),
				Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
				DataSystem: ldcomponents.DataSystem().Custom().
					Initializers(
						ldfiledatav2.DataSource().FilePaths(filename).AsInitializer(),
						ldfiledatav2.DataSource().FilePaths(filename+".does-not-exist").AsInitializer(),
					).
					Synchronizers(
						ldcomponents.StreamingDataSourceV2().BaseURI(server.URL),
					),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			assert.True(t, client.Initialized())
			assert.True(t, logCapture.HasMessageMatch(ldlog.Warn, "Initializer FileDataSynchronizer failed"),
				"the second initializer should have run and failed")

			value, _ := client.BoolVariation("flag-from-file", testUser, false)
			assert.True(t, value)
		})
	})
}

// hangingStreamHandler accepts a connection and never responds.
var hangingStreamHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	<-r.Context().Done()
})

// fileDataWithOneFlag defines flag-from-file, which evaluates to true.
var fileDataWithOneFlag = []byte(`{"flags": {"flag-from-file": {"on": true, "fallthrough": {"variation": 0}, ` +
	`"variations": [true]}}, "segments": {}}`)

// healthyStreamHandler serves alwaysTrueFlag as a full transfer with a selector.
func healthyStreamHandler() http.Handler {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)
	handler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)
	return handler
}

// fetchInitializer is a DataInitializer with a canned Fetch result.
type fetchInitializer struct {
	name  string
	fetch func(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, bool, error)
}

func (i *fetchInitializer) Name() string { return i.name }

func (i *fetchInitializer) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, bool, error) {
	return i.fetch(ds, ctx)
}

func initializerReturning(
	name string,
	fetch func(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, bool, error),
) subsystems.ComponentConfigurer[subsystems.DataInitializer] {
	return mocks.SingleComponentConfigurer[subsystems.DataInitializer]{
		Instance: &fetchInitializer{name: name, fetch: fetch},
	}
}

func noDataBasis() *subsystems.Basis {
	return &subsystems.Basis{ChangeSet: *subsystems.NewChangeSetBuilder().NoChanges()}
}

func malformedBasis(t *testing.T) *subsystems.Basis {
	builder := subsystems.NewChangeSetBuilder().Start(subsystems.ServerIntent{Payload: subsystems.Payload{
		ID: "fake-id", Target: 1, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
	}})
	builder.AddPut(subsystems.FlagKind, "bad", 1, json.RawMessage(`"this is not a flag object"`))
	changeSet, err := builder.Finish(subsystems.NoSelector())
	require.NoError(t, err)
	return &subsystems.Basis{ChangeSet: *changeSet}
}

// An initializer that returns a basis with no data has not provided data. Initialization must
// neither complete nor fail on that result; the synchronizer decides.
func TestFDV2InitializerWithNoDataDefersToSynchronizer(t *testing.T) {
	noData := initializerReturning("no-data",
		func(subsystems.DataSelector, context.Context) (*subsystems.Basis, bool, error) {
			return noDataBasis(), false, nil
		})

	httphelpers.WithServer(healthyStreamHandler(), func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:  ldcomponents.NoEvents(),
			Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().Custom().
				Initializers(noData).
				Synchronizers(ldcomponents.StreamingDataSourceV2().BaseURI(server.URL)),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.True(t, logCapture.HasMessageMatch(ldlog.Warn, "Initializer no-data returned no usable data"))
		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

// An initializer whose data cannot be applied has not provided data. The synchronizer decides.
func TestFDV2InitializerWithMalformedDataDefersToSynchronizer(t *testing.T) {
	malformed := initializerReturning("malformed",
		func(subsystems.DataSelector, context.Context) (*subsystems.Basis, bool, error) {
			return malformedBasis(t), false, nil
		})

	httphelpers.WithServer(healthyStreamHandler(), func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:  ldcomponents.NoEvents(),
			Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().Custom().
				Initializers(malformed).
				Synchronizers(ldcomponents.StreamingDataSourceV2().BaseURI(server.URL)),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.True(t, logCapture.HasMessageMatch(ldlog.Warn, "Initializer malformed returned no usable data"))
		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

// An initializer response that carries data and an FDv1 fallback directive completes
// initialization with that data, before the FDv1 synchronizer has delivered anything.
func TestFDV2FallbackDirectiveWithDataCompletesInitialization(t *testing.T) {
	payload := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag).ToInitializerPayload(subsystems.NoSelector())
	header := http.Header{"X-LD-FD-Fallback": []string{"true"}}
	pollHandler, pollRequests := httphelpers.RecordingHandler(httphelpers.HandlerWithJSONResponse(payload, header))

	httphelpers.WithServer(pollHandler, func(pollServer *httptest.Server) {
		httphelpers.WithServer(hangingStreamHandler, func(hangingServer *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				Events:  ldcomponents.NoEvents(),
				Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
				DataSystem: ldcomponents.DataSystem().Custom().
					Initializers(ldcomponents.PollingDataSourceV2().BaseURI(pollServer.URL).AsInitializer()).
					Synchronizers(ldcomponents.StreamingDataSourceV2().BaseURI(hangingServer.URL)).
					FDv1CompatibleSynchronizer(ldcomponents.FDv1PollingDataSourceV2().BaseURI(hangingServer.URL)),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			<-pollRequests
			require.NoError(t, err)
			defer client.Close()

			assert.True(t, client.Initialized())
			assert.Equal(t, interfaces.DataSourceStateValid, client.GetDataSourceStatusProvider().GetStatus().State)
			value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
			assert.True(t, value)
		})
	})
}

// An FDv1 fallback directive that arrives with a basis that has no data does not complete
// initialization. The FDv1 synchronizer decides.
func TestFDV2FallbackDirectiveWithNoDataDefersToFDv1Synchronizer(t *testing.T) {
	dataV1 := ldservices.NewServerSDKData().Flags(alwaysFalseFlag)
	fallbackWithNoData := initializerReturning("fallback-no-data",
		func(subsystems.DataSelector, context.Context) (*subsystems.Basis, bool, error) {
			return noDataBasis(), true, nil
		})
	pollV1Handler, pollV1Requests := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(dataV1))

	httphelpers.WithServer(pollV1Handler, func(pollV1Server *httptest.Server) {
		httphelpers.WithServer(hangingStreamHandler, func(hangingServer *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				Events:  ldcomponents.NoEvents(),
				Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
				DataSystem: ldcomponents.DataSystem().Custom().
					Initializers(fallbackWithNoData).
					Synchronizers(ldcomponents.StreamingDataSourceV2().BaseURI(hangingServer.URL)).
					FDv1CompatibleSynchronizer(ldcomponents.FDv1PollingDataSourceV2().BaseURI(pollV1Server.URL)),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			<-pollV1Requests
			require.NoError(t, err)
			defer client.Close()

			value, _ := client.BoolVariation(alwaysFalseFlag.Key, testUser, true)
			assert.False(t, value)
		})
	})
}

// A configuration with only initializers completes initialization from initializer data, even
// though that data has no selector and no synchronizer will ever refresh it.
func TestFDV2InitializerOnlyConfigurationCompletesInitialization(t *testing.T) {
	testHelpers.WithTempFileData(fileDataWithOneFlag, func(filename string) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:  ldcomponents.NoEvents(),
			Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().Custom().
				Initializers(ldfiledatav2.DataSource().FilePaths(filename).AsInitializer()),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.True(t, client.Initialized())
		assert.Equal(t, interfaces.DataSourceStateValid, client.GetDataSourceStatusProvider().GetStatus().State)
		value, _ := client.BoolVariation("flag-from-file", testUser, false)
		assert.True(t, value)
	})
}

// A synchronizer that delivers data without a selector completes initialization.
func TestFDV2SelectorlessSynchronizerCompletesInitialization(t *testing.T) {
	testHelpers.WithTempFileData(fileDataWithOneFlag, func(filename string) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:  ldcomponents.NoEvents(),
			Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().Custom().
				Synchronizers(ldfiledatav2.DataSource().FilePaths(filename)),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.True(t, client.Initialized())
		assert.Equal(t, interfaces.DataSourceStateValid, client.GetDataSourceStatusProvider().GetStatus().State)
		value, _ := client.BoolVariation("flag-from-file", testUser, false)
		assert.True(t, value)
	})
}

// When an earlier initializer applied data and a later one signals an FDv1 fallback without data
// of its own, initialization completes with the earlier data instead of waiting for the FDv1
// synchronizer.
func TestFDV2FallbackDirectiveAfterEarlierInitializerDataCompletesInitialization(t *testing.T) {
	fallbackWithoutData := initializerReturning("fallback-without-data",
		func(subsystems.DataSelector, context.Context) (*subsystems.Basis, bool, error) {
			return nil, true, errors.New("server requested fallback")
		})

	testHelpers.WithTempFileData(fileDataWithOneFlag, func(filename string) {
		httphelpers.WithServer(hangingStreamHandler, func(hangingServer *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				Events:  ldcomponents.NoEvents(),
				Logging: ldcomponents.Logging().Loggers(logCapture.Loggers),
				DataSystem: ldcomponents.DataSystem().Custom().
					Initializers(
						ldfiledatav2.DataSource().FilePaths(filename).AsInitializer(),
						fallbackWithoutData,
					).
					Synchronizers(ldcomponents.StreamingDataSourceV2().BaseURI(hangingServer.URL)).
					FDv1CompatibleSynchronizer(ldcomponents.FDv1PollingDataSourceV2().BaseURI(hangingServer.URL)),
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			assert.True(t, client.Initialized())
			assert.Equal(t, interfaces.DataSourceStateValid, client.GetDataSourceStatusProvider().GetStatus().State)
			value, _ := client.BoolVariation("flag-from-file", testUser, false)
			assert.True(t, value)
		})
	})
}

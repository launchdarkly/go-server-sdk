package ldclient

import (
	"crypto/x509"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFDV2DefaultDataSourceIsStreaming(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payloads: []fdv2proto.Payload{
			{ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing"},
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(protocol.Next())

	protocol.Enqueue(streamSender)

	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.DumpIfTestFailed(t)

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem:       ldcomponents.DataSystem().Default(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		assert.True(t, client.Initialized())
	})
}

func TestFDV2ClientStartsInStreamingMode(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payloads: []fdv2proto.Payload{
			{ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing"},
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.DumpIfTestFailed(t)

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem:       ldcomponents.DataSystem().Streaming(),
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

func TestFDV2ClientRetriesConnectionInStreamingModeWithNonFatalError(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payloads: []fdv2proto.Payload{
			{ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing"},
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	failThenSucceedHandler := httphelpers.SequentialHandler(httphelpers.HandlerWithStatus(503), streamHandler)
	handler, requestsCh := httphelpers.RecordingHandler(failThenSucceedHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem:       ldcomponents.DataSystem().Streaming(),
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

func TestFDV2ClientFailsToStartInPollingModeWith401Error(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(pollServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSystem:       ldcomponents.DataSystem().Polling(),
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Polling: pollServer.URL},
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

func TestFDV2ClientUsesCustomTLSConfiguration(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payloads: []fdv2proto.Payload{
			{ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing"},
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(protocol.Next())
	protocol.Enqueue(streamSender)

	httphelpers.WithSelfSignedServer(streamHandler, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
		config := Config{
			Events:           ldcomponents.NoEvents(),
			HTTP:             ldcomponents.HTTPConfiguration().CACert(certData),
			Logging:          ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: server.URL},
			DataSystem:       ldcomponents.DataSystem().Streaming(),
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*50000)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

func TestFDV2ClientStartupTimesOut(t *testing.T) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payloads: []fdv2proto.Payload{
			{ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing"},
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)

	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(protocol.Next())
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
			DataSystem:       ldcomponents.DataSystem().Streaming(),
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

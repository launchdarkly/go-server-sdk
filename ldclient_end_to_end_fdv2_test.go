package ldclient

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
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
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(datasourcev2.ServerIntent{Payloads: []datasourcev2.Payload{
			{ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing"},
		}}).
		WithPutObjects(data.ToBaseObjects()).
		WithTransferred()

	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(protocol.Next())

	protocol.Enqueue(streamSender)

	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.DumpIfTestFailed(t)

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem:       ldcomponents.DataSystem().DefaultMode(),
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
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)

	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(datasourcev2.ServerIntent{Payloads: []datasourcev2.Payload{
			{ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing"},
		}}).
		WithPutObjects(data.ToBaseObjects()).
		WithTransferred()

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
			DataSystem:       ldcomponents.DataSystem().StreamingMode(),
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

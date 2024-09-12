package ldclient

import (
	"encoding/json"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"
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
	requireIntent := func(t *testing.T, code string, reason string) httphelpers.SSEEvent {
		intent := datasourcev2.ServerIntent{Payloads: []datasourcev2.Payload{
			{ID: "fake-id", Target: 0, Code: code, Reason: reason},
		}}
		intentData, err := json.Marshal(intent)
		require.NoError(t, err)
		return httphelpers.SSEEvent{
			Event: "server-intent",
			Data:  string(intentData),
		}
	}

	requireTransferred := func(t *testing.T) httphelpers.SSEEvent {
		type payloadTransferred struct {
			State   string `json:"state"`
			Version int    `json:"version"`
		}
		transferredData, err := json.Marshal(payloadTransferred{State: "[p:17YNC7XBH88Y6RDJJ48EKPCJS7:53]", Version: 1})
		require.NoError(t, err)
		return httphelpers.SSEEvent{
			Event: "payload-transferred",
			Data:  string(transferredData),
		}
	}

	intent := requireIntent(t, "xfer-full", "payload-missing")

	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)

	streamHandler, streamSender := ldservices.ServerSideStreamingServiceHandler(intent)
	for _, object := range data.ToPutObjects() {
		streamSender.Enqueue(object)
	}
	streamSender.Enqueue(requireTransferred(t))

	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.DumpIfTestFailed(t)

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSystem:       ldcomponents.DataSystem(),
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

package ldservices

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helpers "github.com/launchdarkly/go-test-helpers/v2"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
)

func TestServerSideStreamingServiceHandler(t *testing.T) {
	initialEvent := httphelpers.SSEEvent{Event: "put", Data: "my data"}
	handler, stream := ServerSideStreamingServiceHandler(initialEvent)
	defer stream.Close()

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		t.Run("sends events", func(t *testing.T) {
			resp, err := http.DefaultClient.Get(server.URL + ServerSideSDKStreamingPath)
			require.NoError(t, err)
			defer resp.Body.Close()

			expected := initialEvent.Bytes()
			assert.Equal(t, string(expected), string(helpers.ReadWithTimeout(resp.Body, len(expected), time.Second)))

			event2 := httphelpers.SSEEvent{Event: "patch", Data: "more data"}
			stream.Send(event2)

			expected = event2.Bytes()
			assert.Equal(t, string(expected), string(helpers.ReadWithTimeout(resp.Body, len(expected), time.Second)))
		})

		t.Run("returns 404 for wrong URL", func(t *testing.T) {
			resp, err := http.DefaultClient.Get(server.URL + "/some/other/path")
			assert.NoError(t, err)
			assert.Equal(t, 404, resp.StatusCode)
		})

		t.Run("returns 405 for wrong method", func(t *testing.T) {
			resp, err := http.DefaultClient.Post(server.URL+ServerSideSDKStreamingPath, "text/plain", bytes.NewBufferString("hello"))
			assert.NoError(t, err)
			assert.Equal(t, 405, resp.StatusCode)
		})
	})
}

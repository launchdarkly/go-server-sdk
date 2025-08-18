package ldmiddleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

type recordingHook struct {
	ldhooks.Unimplemented
	events []ldhooks.TrackSeriesContext
}

func (h *recordingHook) Metadata() ldhooks.Metadata { return ldhooks.NewMetadata("rec") }
func (h *recordingHook) AfterTrack(_ context.Context, sc ldhooks.TrackSeriesContext) error {
	h.events = append(h.events, sc)
	return nil
}

func makeClientWithHook(t *testing.T, hook ldhooks.Hook) *ld.LDClient {
	t.Helper()
	config := ld.Config{
		DataSource:       ldcomponents.ExternalUpdatesOnly(),
		Events:           ldcomponents.SendEvents().FlushInterval(time.Hour),
		DiagnosticOptOut: true,
		Hooks:            []ldhooks.Hook{hook},
	}
	client, err := ld.MakeCustomClient("sdk-key", config, time.Second)
	if err != nil {
		t.Fatalf("failed to make client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestAddScopedClientForRequest_SetsScopedClientAndContext(t *testing.T) {
	client := makeClientWithHook(t, &recordingHook{})
	handler := AddScopedClientForRequest(client)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc, ok := ld.GetScopedClient(r.Context())
		if !ok {
			t.Fatalf("scoped client not found in request context")
		}
		ctx := sc.CurrentContext()
		assert.Equal(t, "request", string(ctx.Kind()))
		assert.NotEmpty(t, ctx.Key())
		assert.Equal(t, "GET", ctx.GetValue("method").StringValue())
		assert.Equal(t, "test", ctx.GetValue("host").StringValue())
		assert.NotEmpty(t, ctx.GetValue("userAgent").StringValue())
		assert.Equal(t, "user-agent/1.0", ctx.GetValue("userAgent").StringValue())
		assert.Equal(t, "/path", ctx.GetValue("path").StringValue())
		assert.Equal(t, "http", ctx.GetValue("scheme").StringValue())
		assert.Equal(t, "q=1", ctx.GetValue("query").StringValue())
		assert.Equal(t, "HTTP/1.1", ctx.GetValue("proto").StringValue())
		assert.NotEmpty(t, ctx.GetValue("remoteAddr").StringValue())
		assert.Equal(t, r.RemoteAddr, ctx.GetValue("remoteAddr").StringValue())
		w.WriteHeader(204)
	}))
	req := httptest.NewRequest("GET", "http://test/path?q=1", nil)
	req.Header.Set("User-Agent", "user-agent/1.0")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != 204 {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
}

func TestTrackTiming_AfterTrackReceivesDurationMetric(t *testing.T) {
	rec := &recordingHook{}
	client := makeClientWithHook(t, rec)

	leaf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(204)
	})
	handler := AddScopedClientForRequest(client)(TrackTiming(leaf))

	req := httptest.NewRequest("GET", "http://test/path", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, 204, rr.Code)
	assert.Equal(t, 1, len(rec.events))
	e := rec.events[0]
	assert.Equal(t, "http.request.duration_ms", e.Key())
	assert.NotNil(t, e.MetricValue())
	assert.Greater(t, *e.MetricValue(), 0.0)
}

func TestTrackErrorResponses_AfterTrackReceivesErrorKeys(t *testing.T) {
	rec := &recordingHook{}
	client := makeClientWithHook(t, rec)

	for _, status := range []int{404, 503} {
		t.Run(fmt.Sprintf("status %d", status), func(t *testing.T) {
			rec.events = nil
			leaf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
			handler := AddScopedClientForRequest(client)(TrackErrorResponses(leaf))

			req := httptest.NewRequest("GET", "http://test/path", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			assert.Equal(t, status, rr.Code)
			assert.Equal(t, 1, len(rec.events))
			want := "http.response.5xx"
			if status < 500 {
				want = "http.response.4xx"
			}
			assert.Equal(t, want, rec.events[0].Key())
		})
	}

	t.Run("no events when no error", func(t *testing.T) {
		rec.events = nil
		leaf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
		handler := AddScopedClientForRequest(client)(TrackErrorResponses(leaf))

		req := httptest.NewRequest("GET", "http://test/path", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, 200, rr.Code)
		assert.Equal(t, 0, len(rec.events))
	})
}

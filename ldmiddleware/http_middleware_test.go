package ldmiddleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
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
		assert.Equal(t, r.Method, ctx.GetValue("method").StringValue())
		assert.Equal(t, r.URL.Path, ctx.GetValue("path").StringValue())
		assert.Equal(t, r.UserAgent(), ctx.GetValue("userAgent").StringValue())
		assert.Equal(t, r.URL.Scheme, ctx.GetValue("scheme").StringValue())
		assert.Equal(t, r.URL.RawQuery, ctx.GetValue("query").StringValue())
		assert.Equal(t, r.Proto, ctx.GetValue("proto").StringValue())
		assert.Equal(t, r.Host, ctx.GetValue("host").StringValue())
		assert.Equal(t, r.RemoteAddr, ctx.GetValue("remoteAddr").StringValue())
		w.WriteHeader(204)
	}))
	req := httptest.NewRequest("GET", "http://test/path?q=1", nil)
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
	if rr.Code != 204 {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 track event, got %d", len(rec.events))
	}
	e := rec.events[0]
	if e.Key() != "http.request.duration_ms" {
		t.Fatalf("unexpected key: %s", e.Key())
	}
	if e.MetricValue() == nil || *e.MetricValue() <= 0 {
		t.Fatalf("expected positive metric value")
	}
}

func TestTrackErrorResponses_AfterTrackReceivesErrorKeys(t *testing.T) {
	rec := &recordingHook{}
	client := makeClientWithHook(t, rec)

	for _, status := range []int{404, 503} {
		rec.events = nil
		leaf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		handler := AddScopedClientForRequest(client)(TrackErrorResponses(leaf))

		req := httptest.NewRequest("GET", "http://test/path", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != status {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
		if len(rec.events) != 1 {
			t.Fatalf("expected 1 track event, got %d", len(rec.events))
		}
		want := "http.response.5xx"
		if status < 500 {
			want = "http.response.4xx"
		}
		if rec.events[0].Key() != want {
			t.Fatalf("unexpected key: %s", rec.events[0].Key())
		}
	}
}

package ldmiddleware

import (
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"

	"github.com/launchdarkly/go-sdk-common/v4/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// RequestKeyFunc allows callers to override the request context key for the LDContext.
// Return (key, true) to use the provided key; return ("", false) to fall back to the default UUID key.
type RequestKeyFunc func(r *http.Request) (string, bool)

// AddScopedClientForRequest returns a net/http middleware that, for each incoming request,
// creates an LDScopedClient seeded with a `request`-kind LDContext populated with useful
// HTTP request attributes (e.g., method, path, host, userAgent), and stores it in the
// request's Go context. Downstream handlers can retrieve it via ld.GetScopedClient.
func AddScopedClientForRequest(client *ld.LDClient) func(next http.Handler) http.Handler {
	return AddScopedClientForRequestWithKeyFn(client, nil)
}

// AddScopedClientForRequestWithKeyFn is like AddScopedClientForRequest, but allows providing a function to override
// the context key used for the `request`-kind LDContext. If the function returns ok=false or an empty key,
// a random UUID will be used.
func AddScopedClientForRequestWithKeyFn(
	client *ld.LDClient, keyFn RequestKeyFunc,
) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Determine request context key
			requestKey := ""
			if keyFn != nil {
				if k, ok := keyFn(r); ok && k != "" {
					requestKey = k
				}
			}
			if requestKey == "" {
				requestKey = uuid.New().String()
			}

			b := ldcontext.NewBuilder(requestKey).Kind("ld_request").Anonymous(true)
			b.SetString("method", r.Method)
			b.SetString("host", r.Host)
			b.SetString("userAgent", r.UserAgent())
			if r.URL != nil {
				b.SetString("path", r.URL.Path)
				b.SetString("scheme", r.URL.Scheme)
				b.SetString("query", r.URL.RawQuery)
			}
			b.SetString("proto", r.Proto)
			b.SetString("remoteAddr", r.RemoteAddr)
			requestCtx := b.Build()

			scoped := ld.NewScopedClient(client, requestCtx)
			ctxWithScoped := ld.GoContextWithScopedClient(r.Context(), scoped)

			next.ServeHTTP(w, r.WithContext(ctxWithScoped))
		})
	}
}

// TrackTiming sends a LD event "http.request.duration_ms" with the duration of the request in milliseconds.
// This middleware must be after AddScopedClientForRequest in the middleware chain, as it uses the scoped client
// from the Go context.
//
// The timing event will include all LaunchDarkly contexts added to the scoped client. You may add more
// contexts to the scoped client _during_ the request, and they will be included in the timing event sent
// when the request completes.
func TrackTiming(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(startTime)
		scoped, ok := ld.GetScopedClient(r.Context())
		if !ok {
			return
		}
		_ = scoped.TrackMetric("http.request.duration_ms", float64(duration.Milliseconds()), ldvalue.Null())
	})
}

// TrackErrorResponses sends a LD event "http.response.4xx" or "http.response.5xx" if the response code is 4xx or 5xx.
// This middleware must be after AddScopedClientForRequest in the middleware chain, as it uses the scoped client
// from the Go context.
//
// The error event will include all LaunchDarkly contexts added to the scoped client. You may add more
// contexts to the scoped client _during_ the request, and they will be included in the error event sent
// when the request completes.
func TrackErrorResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics := httpsnoop.CaptureMetrics(next, w, r)
		if metrics.Code < 400 {
			return
		}
		scoped, ok := ld.GetScopedClient(r.Context())
		if !ok {
			return
		}
		if metrics.Code < 500 {
			_ = scoped.TrackEvent("http.response.4xx")
			return
		}
		_ = scoped.TrackEvent("http.response.5xx")
	})
}

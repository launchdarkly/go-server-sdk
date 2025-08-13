package ldmiddleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// RequestKeyFunc allows callers to override the request context key for the LDContext.
// Return (key, true) to use the provided key; return ("", false) to fall back to the default UUID key.
type RequestKeyFunc func(r *http.Request) (string, bool)

// AddRequestScopedClient returns a net/http middleware that, for each incoming request,
// creates an LDScopedClient seeded with a `request`-kind LDContext populated with useful
// HTTP request attributes (e.g., method, path, host, userAgent), and stores it in the
// request's Go context. Downstream handlers can retrieve it via ld.GetScopedClient.
func AddRequestScopedClient(client *ld.LDClient) func(next http.Handler) http.Handler {
	return AddRequestScopedClientWithKeyFn(client, nil)
}

// AddRequestScopedClientWithKeyFn is like AddRequestScopedClient, but allows providing a function to override
// the context key used for the `request`-kind LDContext. If the function returns ok=false or an empty key,
// a random UUID will be used.
func AddRequestScopedClientWithKeyFn(client *ld.LDClient, keyFn RequestKeyFunc) func(next http.Handler) http.Handler {
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

			b := ldcontext.NewBuilder(requestKey).Kind("request").Anonymous(true)
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

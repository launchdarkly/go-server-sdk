package ldmiddleware

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// AddRequestScopedClient returns a net/http middleware that, for each incoming request,
// creates an LDScopedClient seeded with a `request`-kind LDContext and stores it in the
// request's Go context. Downstream handlers can retrieve it via ld.GetScopedClient.
func AddRequestScopedClient(client *ld.LDClient) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use a UUID to identify the request context key.
			requestKey := uuid.New().String()
			requestCtx := ldcontext.NewWithKind("request", requestKey)

			scoped := ld.NewScopedClient(client, requestCtx)
			ctxWithScoped := ld.GoContextWithScopedClient(r.Context(), scoped)

			next.ServeHTTP(w, r.WithContext(ctxWithScoped))
		})
	}
}

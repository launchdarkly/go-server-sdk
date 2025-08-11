package ldclient

import "context"

type scopedClientKey struct{}

// GoContextWithScopedClient adds a scoped client to the Go context. This can be
// used to pass a scoped client to a function or goroutine that might not
// otherwise have access to it:
//
//	scopedClient := ld.NewScopedClient(client, ldUserContext)
//	ctx := ld.GoContextWithScopedClient(context.Background(), scopedClient)
//	otherFunction(ctx)
func GoContextWithScopedClient(ctx context.Context, client *LDScopedClient) context.Context {
	return context.WithValue(ctx, scopedClientKey{}, client)
}

// GetScopedClient retrieves a scoped client from the Go context that was set
// with GoContextWithScopedClient, if present. If not present, returns nil and
// false.
//
//	func logicWithFeatureFlag(ctx context.Context) {
//		scopedClient, ok := ld.GetScopedClient(ctx)
//		isFeatureEnabled := false // default value if scoped client is not available
//		if ok {
//			isFeatureEnabled, err = scopedClient.BoolVariation("my-flag", false)
//			// handle err as appropriate...
//		}
//	}
func GetScopedClient(ctx context.Context) (*LDScopedClient, bool) {
	client, ok := ctx.Value(scopedClientKey{}).(*LDScopedClient)
	return client, ok
}

// MustGetScopedClient retrieves a scoped client from the Go context that was set
// with GoContextWithScopedClient, or panics if not present.
//
//	func logicWithFeatureFlag(ctx context.Context) {
//		scopedClient := ld.MustGetScopedClient(ctx)
//		isFeatureEnabled, err := scopedClient.BoolVariation("my-flag", false)
//		// handle err as appropriate...
//	}
func MustGetScopedClient(ctx context.Context) *LDScopedClient {
	client, ok := GetScopedClient(ctx)
	if !ok {
		panic("No scoped client found in context")
	}
	return client
}

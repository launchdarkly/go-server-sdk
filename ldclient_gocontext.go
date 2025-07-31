package ldclient

import "context"

type scopedClientKey struct{}

func GoContextWithScopedClient(ctx context.Context, client *LDScopedClient) context.Context {
	return context.WithValue(ctx, scopedClientKey{}, client)
}

func GetScopedClient(ctx context.Context) (*LDScopedClient, bool) {
	client, ok := ctx.Value(scopedClientKey{}).(*LDScopedClient)
	return client, ok
}

func MustGetScopedClient(ctx context.Context) *LDScopedClient {
	client, ok := GetScopedClient(ctx)
	if !ok {
		panic("No scoped client found in context")
	}
	return client
}

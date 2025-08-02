package ldclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetScopedClient(t *testing.T) {
	t.Run("returns client from context", func(t *testing.T) {
		origCtx := context.Background()
		sc := &LDScopedClient{}

		newCtx := GoContextWithScopedClient(origCtx, sc)
		retrieved, ok := GetScopedClient(newCtx)

		assert.True(t, ok, "expected to find scoped client in context")
		assert.Equal(t, sc, retrieved, "retrieved client should match original")
	})

	t.Run("returns nil when not present", func(t *testing.T) {
		retrieved, ok := GetScopedClient(context.Background())
		assert.False(t, ok, "should not find scoped client in empty context")
		assert.Nil(t, retrieved, "retrieved client should be nil when not present")
	})
}

func TestMustGetScopedClient(t *testing.T) {
	sc := &LDScopedClient{}
	ctxWith := GoContextWithScopedClient(context.Background(), sc)

	// Should return the client without panicking when present
	assert.Equal(t, sc, MustGetScopedClient(ctxWith))

	// Should panic when the client is not present
	assert.Panics(t, func() {
		MustGetScopedClient(context.Background())
	})
}

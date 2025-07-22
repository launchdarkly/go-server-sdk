package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
)

func TestScopedClientCurrentContext(t *testing.T) {
	ldctx := ldcontext.New("user1")
	c := makeTestClient().ForContext(ldctx)

	assert.Equal(t, ldctx, c.CurrentContext())
}

func TestScopedClientCollectsContexts(t *testing.T) {
	ldctx1 := ldcontext.NewWithKind("foo", "foo1")
	ldctx2 := ldcontext.NewMulti(
		ldcontext.NewWithKind("bar", "bar1"),
		ldcontext.NewWithKind("baz", "baz1"),
	)
	ldctx3 := ldcontext.NewWithKind("qux", "qux1")
	ldctx4 := ldcontext.NewMulti(
		ldcontext.NewWithKind("quux", "quux1"),
		ldcontext.NewWithKind("quuz", "quuz1"),
	)
	dupeCtx := ldcontext.NewWithKind("foo", "foo2")

	t.Run("adding contexts happy path", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		c := makeTestClientWithConfig(func(config *Config) {
			config.Logging = ldcomponents.Logging().Loggers(logCapture.Loggers)
		}).ForContext(ldctx1, ldctx2)

		c.AddContext(ldctx3, ldctx4)

		assert.Equal(t, ldcontext.NewMulti(ldctx1, ldctx2, ldctx3, ldctx4), c.CurrentContext())
		assert.Empty(t, logCapture.GetOutput(ldlog.Warn))
	})

	t.Run("adding duplicate context kinds", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		c := makeTestClientWithConfig(func(config *Config) {
			config.Logging = ldcomponents.Logging().Loggers(logCapture.Loggers)
		}).ForContext(ldctx1)

		c.AddContext(dupeCtx)

		assert.Equal(t, ldcontext.NewMulti(ldctx1), c.CurrentContext())
		logCapture.AssertMessageMatch(t, true, ldlog.Warn, "Tried to add a duplicate foo context to LDScopedClient")
	})

	t.Run("overwriting contexts", func(t *testing.T) {
		c := makeTestClient().ForContext(ldctx1, ldctx2, ldctx3, ldctx4)

		c.OverwriteContextByKind(dupeCtx)
		c.OverwriteContextByKind(ldctx2, ldctx3)

		assert.Equal(t, ldcontext.NewMulti(ldctx2, ldctx3, ldctx4, dupeCtx), c.CurrentContext())
	})
}

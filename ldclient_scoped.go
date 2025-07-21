package ldclient

import (
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
)

type LDScopedClient struct {
	sync.Mutex
	client *LDClient

	contexts map[ldcontext.Kind]ldcontext.Context

	// Caching mechanism to avoid rebuilding the context every time
	context ldcontext.Context
	rebuild bool
}

func (c *LDClient) ForContext(contexts ...ldcontext.Context) *LDScopedClient {
	cc := &LDScopedClient{
		client:   c,
		contexts: make(map[ldcontext.Kind]ldcontext.Context),
		rebuild:  true,
	}
	cc.AddContext(contexts...)
	return cc
}

func (c *LDScopedClient) AddContext(contexts ...ldcontext.Context) {
	c.Lock()
	defer c.Unlock()
	c.rebuild = true

	for _, ctx := range contexts {
		if ctx.Multiple() {
			c.AddContext(ctx.GetAllIndividualContexts(nil)...)
			continue
		}
		if _, ok := c.contexts[ctx.Kind()]; ok {
			c.client.loggers.Warnf("Tried to add a duplicate %s context to LDScopedClient", ctx.Kind())
			continue
		}
		c.contexts[ctx.Kind()] = ctx
	}
}

func (c *LDScopedClient) OverwriteContextByKind(contexts ...ldcontext.Context) {
	c.Lock()
	defer c.Unlock()
	c.rebuild = true

	for _, ctx := range contexts {
		if ctx.Multiple() {
			c.OverwriteContextByKind(ctx.GetAllIndividualContexts(nil)...)
			continue
		}
		c.contexts[ctx.Kind()] = ctx
	}
}

func (c *LDScopedClient) CurrentContext() ldcontext.Context {
	c.Lock()
	defer c.Unlock()
	if !c.rebuild {
		return c.context
	}
	c.rebuild = false
	b := ldcontext.NewMultiBuilder()
	for _, ctx := range c.contexts {
		b.Add(ctx)
	}
	c.context = b.Build()
	return c.context
}

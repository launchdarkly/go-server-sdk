package ldclient

import (
	gocontext "context"
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces/flagstate"
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

func (c *LDScopedClient) Client() *LDClient {
	return c.client
}

// Contextual methods: equivalent to calling the same method on the underlying client with the current context

func (c *LDScopedClient) BoolVariation(key string, defaultVal bool) (bool, error) {
	return c.client.BoolVariation(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) BoolVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal bool,
) (bool, error) {
	return c.client.BoolVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) BoolVariationDetail(key string, defaultVal bool) (bool, ldreason.EvaluationDetail, error) {
	return c.client.BoolVariationDetail(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) BoolVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal bool,
) (bool, ldreason.EvaluationDetail, error) {
	return c.client.BoolVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) IntVariation(key string, defaultVal int) (int, error) {
	return c.client.IntVariation(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) IntVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal int,
) (int, error) {
	return c.client.IntVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) IntVariationDetail(key string, defaultVal int) (int, ldreason.EvaluationDetail, error) {
	return c.client.IntVariationDetail(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) IntVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal int,
) (int, ldreason.EvaluationDetail, error) {
	return c.client.IntVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) Float64Variation(key string, defaultVal float64) (float64, error) {
	return c.client.Float64Variation(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) Float64VariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal float64,
) (float64, error) {
	return c.client.Float64VariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) Float64VariationDetail(key string, defaultVal float64) (float64, ldreason.EvaluationDetail, error) {
	return c.client.Float64VariationDetail(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) Float64VariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal float64,
) (float64, ldreason.EvaluationDetail, error) {
	return c.client.Float64VariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) StringVariation(key string, defaultVal string) (string, error) {
	return c.client.StringVariation(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) StringVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal string,
) (string, error) {
	return c.client.StringVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) StringVariationDetail(key string, defaultVal string) (string, ldreason.EvaluationDetail, error) {
	return c.client.StringVariationDetail(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) StringVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal string,
) (string, ldreason.EvaluationDetail, error) {
	return c.client.StringVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) JSONVariation(key string, defaultVal ldvalue.Value) (ldvalue.Value, error) {
	return c.client.JSONVariation(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) JSONVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal ldvalue.Value,
) (ldvalue.Value, error) {
	return c.client.JSONVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) JSONVariationDetail(key string, defaultVal ldvalue.Value) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	return c.client.JSONVariationDetail(key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) JSONVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal ldvalue.Value,
) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	return c.client.JSONVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

func (c *LDScopedClient) MigrationVariation(
	key string,
	defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return c.client.MigrationVariation(key, c.CurrentContext(), defaultStage)
}

func (c *LDScopedClient) MigrationVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return c.client.MigrationVariationCtx(ctx, key, c.CurrentContext(), defaultStage)
}

func (c *LDScopedClient) AllFlagsState(
	options ...flagstate.Option,
) flagstate.AllFlags {
	return c.client.AllFlagsState(c.CurrentContext(), options...)
}

func (c *LDScopedClient) Identify() error {
	return c.client.Identify(c.CurrentContext())
}

func (c *LDScopedClient) TrackEvent(eventName string) error {
	return c.client.TrackEvent(eventName, c.CurrentContext())
}

func (c *LDScopedClient) TrackEventCtx(
	ctx gocontext.Context,
	eventName string,
) error {
	return c.client.TrackEventCtx(ctx, eventName, c.CurrentContext())
}

func (c *LDScopedClient) TrackData(eventName string, data ldvalue.Value) error {
	return c.client.TrackData(eventName, c.CurrentContext(), data)
}

func (c *LDScopedClient) TrackDataCtx(
	ctx gocontext.Context,
	eventName string,
	data ldvalue.Value,
) error {
	return c.client.TrackDataCtx(ctx, eventName, c.CurrentContext(), data)
}

func (c *LDScopedClient) TrackMetric(eventName string, metricValue float64, data ldvalue.Value) error {
	return c.client.TrackMetric(eventName, c.CurrentContext(), metricValue, data)
}

func (c *LDScopedClient) TrackMetricCtx(
	ctx gocontext.Context,
	eventName string,
	metricValue float64,
	data ldvalue.Value,
) error {
	return c.client.TrackMetricCtx(ctx, eventName, c.CurrentContext(), metricValue, data)
}

func (c *LDScopedClient) TrackMigrationOp(event ldevents.MigrationOpEventData) error {
	return c.client.TrackMigrationOp(event)
}

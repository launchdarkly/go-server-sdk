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
)

// LDScopedClient is a wrapper around LDClient that lets you specify the
// evaluation context to be used for all operations, rather than taking the
// evaluation context as a parameter every time you call a method.
//
// A LDScopedClient is created by calling [LDClient.ForContext]. This sets the
// initial context to be used for all operations:
//
//	userContext := ldcontext.New("user-key")
//	scopedClient := client.ForContext(userContext)
//	scopedClient.BoolVariation("flag-key", false)
//
// Scoped contexts are mutable, to facilitate incrementally building up a
// multi-context representing the current scope. You should create a new scoped
// client for each logical scope where the contexts are different. For instance,
// if you are using a scoped client in an HTTP request handler, you should create
// a new scoped client for each request. You should only use the mutable features
// of LDScopedClient to add new contexts, not to change or remove existing data.
// A scoped client is thread-safe, so you can safely use it from multiple
// goroutines.
//
// You may add new contexts with [LDScopedClient.AddContext]. The scoped client's
// contexts so far are combined into a multi-context whenever the scoped client
// is used. This is useful when more contexts become available later in the
// lifecycle of a request. For instance, you might have a `user` context that is
// available early in the request, but you also want to evaluate flags with a
// `company` context that is only available later:
//
//	company := fetchCompanyForUser(user)
//	scopedClient.AddContext(ldcontext.NewWithKind("company", company.Id))
//	scopedClient.BoolVariation("enterprise-features", false)
//
// You can also overwrite a context that was previously added, by calling
// [LDScopedClient.OverwriteContextByKind]. This can be used when an existing
// context needs to be updated with new data:
//
//	entitlements := fetchUserEntitlements(user)
//	fullUserCtx := ldcontext.NewBuilder(user.Id).Set("entitlements", entitlements).Build()
//	scopedClient.OverwriteContextByKind(fullUserCtx)
type LDScopedClient struct {
	sync.Mutex
	client *LDClient

	contexts map[ldcontext.Kind]ldcontext.Context

	// Caching mechanism to avoid rebuilding the context every time
	context ldcontext.Context
	rebuild bool
}

// ForContext creates a new LDScopedClient, a wrapper of LDClient that uses a
// certain LaunchDarkly evaluation context for all operations, like evaluating
// feature flags or sending events. The scoped client supports most of the same
// methods as LDClient, like BoolVariation et al., but without the
// ldcontext.Context parameter.
//
// You may pass a multi-context, or multiple individual contexts, to ForContext.
// All of the contexts passed in will be combined into a multi-context when the
// scoped client is used.
//
// You should create a new scoped client for each logical scope where the
// contexts are isolated from each other, like a web request.
//
// For more info on how to use the scoped client, see the documentation for
// LDScopedClient.
func (client *LDClient) ForContext(contexts ...ldcontext.Context) *LDScopedClient {
	cc := &LDScopedClient{
		client:   client,
		contexts: make(map[ldcontext.Kind]ldcontext.Context),
		rebuild:  true,
	}
	cc.AddContext(contexts...)
	return cc
}

func (c *LDScopedClient) addIndividualContext(context ldcontext.Context) {
	if _, ok := c.contexts[context.Kind()]; ok {
		c.client.loggers.Warnf("Tried to add a duplicate %s context to LDScopedClient", context.Kind())
		return
	}
	c.contexts[context.Kind()] = context
}

// AddContext adds additional evaluation contexts to the scoped client, affecting
// all future operations on it, like flag evaluations and event tracking.
//
// All of the contexts added must have kinds that are not already present. If you
// try to add a context with a duplicate kind, the new context will not be added
// and a warning will be logged. If you want to replace an existing context, use
// [LDScopedClient.OverwriteContextByKind] instead.
//
// The scoped client's contexts so far are combined into a multi-context whenever the
// scoped client is used.
func (c *LDScopedClient) AddContext(contexts ...ldcontext.Context) {
	c.Lock()
	defer c.Unlock()
	c.rebuild = true

	for _, ctx := range contexts {
		if ctx.Multiple() {
			for _, individual := range ctx.GetAllIndividualContexts(nil) {
				c.addIndividualContext(individual)
			}
			continue
		}
		c.addIndividualContext(ctx)
	}
}

func (c *LDScopedClient) overwriteIndividualContextByKind(context ldcontext.Context) {
	c.contexts[context.Kind()] = context
}

// OverwriteContextByKind overwrites an existing context in the scoped client, affecting all future
// operations on it, like flag evaluations and event tracking.
//
// You may pass a multi-context, or multiple individual contexts, to OverwriteContextByKind.
// Any existing contexts with the same kind as any of the new contexts will be overwritten.
//
// If the scoped client had multiple contexts, only the context with the same kind as the new context
// will be overwritten. Any other existing contexts will remain unchanged:
//
//	company := ldcontext.NewWithKind("company", "acme")
//	scopedClient := client.ForContext(company)
//	scopedClient.AddContext(ldcontext.New("user-key"))
//	scopedClient.OverwriteContextByKind(ldcontext.NewWithKind("company", "monsoon"))
//	scopedClient.CurrentContext() // returns a multi-context with "monsoon" and "user-key"
func (c *LDScopedClient) OverwriteContextByKind(contexts ...ldcontext.Context) {
	c.Lock()
	defer c.Unlock()
	c.rebuild = true

	for _, ctx := range contexts {
		if ctx.Multiple() {
			for _, individual := range ctx.GetAllIndividualContexts(nil) {
				c.overwriteIndividualContextByKind(individual)
			}
			continue
		}
		c.overwriteIndividualContextByKind(ctx)
	}
}

// CurrentContext returns the current LaunchDarkly context for the scoped client.
// This is the combination of all the contexts that have been added so far, as a
// multi-context.
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

// Client returns the underlying LDClient that this scoped client wraps.
func (c *LDScopedClient) Client() *LDClient {
	return c.client
}

// Contextual methods: equivalent to calling the same method on the underlying client with the current context

// BoolVariation returns the value of a boolean feature flag for the current
// evaluation context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) BoolVariation(key string, defaultVal bool) (bool, error) {
	return c.client.BoolVariation(key, c.CurrentContext(), defaultVal)
}

// BoolVariationCtx is the same as [LDScopedClient.BoolVariation], but accepts a
// context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) BoolVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal bool,
) (bool, error) {
	return c.client.BoolVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// BoolVariationDetail is the same as [LDScopedClient.BoolVariation], but also
// returns further information about how the value was calculated. The "reason"
// data will also be included in analytics events.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) BoolVariationDetail(key string, defaultVal bool) (bool, ldreason.EvaluationDetail, error) {
	return c.client.BoolVariationDetail(key, c.CurrentContext(), defaultVal)
}

// BoolVariationDetailCtx is the same as [LDScopedClient.BoolVariationCtx], but
// also returns further information about how the value was calculated. The
// "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) BoolVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal bool,
) (bool, ldreason.EvaluationDetail, error) {
	return c.client.BoolVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// IntVariation returns the value of a feature flag (whose variations are
// integers) for the current evaluation context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the
// feature is turned off and has no off variation.
//
// If the flag variation has a numeric value that is not an integer, it is
// rounded toward zero (truncated).
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) IntVariation(key string, defaultVal int) (int, error) {
	return c.client.IntVariation(key, c.CurrentContext(), defaultVal)
}

// IntVariationCtx is the same as [LDScopedClient.IntVariation], but accepts a
// context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) IntVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal int,
) (int, error) {
	return c.client.IntVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// IntVariationDetail is the same as [LDScopedClient.IntVariation], but also
// returns further information about how the value was calculated. The "reason"
// data will also be included in analytics events.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) IntVariationDetail(key string, defaultVal int) (int, ldreason.EvaluationDetail, error) {
	return c.client.IntVariationDetail(key, c.CurrentContext(), defaultVal)
}

// IntVariationDetailCtx is the same as [LDScopedClient.IntVariationCtx], but
// also returns further information about how the value was calculated. The
// "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) IntVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal int,
) (int, ldreason.EvaluationDetail, error) {
	return c.client.IntVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// Float64Variation returns the value of a feature flag (whose variations are
// floats) for the current evaluation context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the
// feature is turned off and has no off variation.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) Float64Variation(key string, defaultVal float64) (float64, error) {
	return c.client.Float64Variation(key, c.CurrentContext(), defaultVal)
}

// Float64VariationCtx is the same as [LDScopedClient.Float64Variation], but
// accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) Float64VariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal float64,
) (float64, error) {
	return c.client.Float64VariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// Float64VariationDetail is the same as [LDScopedClient.Float64Variation], but
// also returns further information about how the value was calculated. The
// "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) Float64VariationDetail(
	key string,
	defaultVal float64,
) (float64, ldreason.EvaluationDetail, error) {
	return c.client.Float64VariationDetail(key, c.CurrentContext(), defaultVal)
}

// Float64VariationDetailCtx is the same as [LDScopedClient.Float64VariationCtx],
// but also returns further information about how the value was calculated. The
// "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) Float64VariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal float64,
) (float64, ldreason.EvaluationDetail, error) {
	return c.client.Float64VariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// StringVariation returns the value of a feature flag (whose variations are
// strings) for the current evaluation context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the
// feature is turned off and has no off variation.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) StringVariation(key string, defaultVal string) (string, error) {
	return c.client.StringVariation(key, c.CurrentContext(), defaultVal)
}

// StringVariationCtx is the same as [LDScopedClient.StringVariation], but
// accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) StringVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal string,
) (string, error) {
	return c.client.StringVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// StringVariationDetail is the same as [LDScopedClient.StringVariation], but
// also returns further information about how the value was calculated. The
// "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) StringVariationDetail(
	key string,
	defaultVal string,
) (string, ldreason.EvaluationDetail, error) {
	return c.client.StringVariationDetail(key, c.CurrentContext(), defaultVal)
}

// StringVariationDetailCtx is the same as [LDScopedClient.StringVariationCtx],
// but also returns further information about how the value was calculated. The
// "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) StringVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal string,
) (string, ldreason.EvaluationDetail, error) {
	return c.client.StringVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// JSONVariation returns the value of a feature flag for the current evaluation
// context, allowing the value to be of any JSON type.
//
// The value is returned as an [ldvalue.Value], which can be inspected or
// converted to other types using methods such as [ldvalue.Value.GetType] and
// [ldvalue.Value.BoolValue]. The defaultVal parameter also uses this type. For
// instance, if the values for this flag are JSON arrays:
//
//	defaultValAsArray := ldvalue.BuildArray().
//	    Add(ldvalue.String("defaultFirstItem")).
//	    Add(ldvalue.String("defaultSecondItem")).
//	    Build()
//	result, err := client.JSONVariation(flagKey, defaultValAsArray)
//	firstItemAsString := result.GetByIndex(0).StringValue() // "defaultFirstItem", etc.
//
// You can also use unparsed json.RawMessage values:
//
//	defaultValAsRawJSON := ldvalue.Raw(json.RawMessage(`{"things":[1,2,3]}`))
//	result, err := client.JSONVariation(flagKey, defaultValAsRawJSON)
//	resultAsRawJSON := result.AsRaw()
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the
// feature is turned off.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) JSONVariation(key string, defaultVal ldvalue.Value) (ldvalue.Value, error) {
	return c.client.JSONVariation(key, c.CurrentContext(), defaultVal)
}

// JSONVariationCtx is the same as [LDScopedClient.JSONVariation], but accepts a
// context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) JSONVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultVal ldvalue.Value,
) (ldvalue.Value, error) {
	return c.client.JSONVariationCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// JSONVariationDetail is the same as [LDScopedClient.JSONVariation], but also
// returns further information about how the value was calculated. The "reason"
// data will also be included in analytics events.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) JSONVariationDetail(
	key string,
	defaultVal ldvalue.Value,
) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	return c.client.JSONVariationDetail(key, c.CurrentContext(), defaultVal)
}

// JSONVariationDetailCtx is the same as [LDScopedClient.JSONVariationCtx], but
// also returns further information about how the value was calculated. The
// "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (c *LDScopedClient) JSONVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	defaultVal ldvalue.Value,
) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	return c.client.JSONVariationDetailCtx(ctx, key, c.CurrentContext(), defaultVal)
}

// MigrationVariation returns the migration stage of the migration feature flag
// for the current evaluation context.
//
// Returns defaultStage if there is an error or if the flag doesn't exist.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) MigrationVariation(
	key string,
	defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return c.client.MigrationVariation(key, c.CurrentContext(), defaultStage)
}

// MigrationVariationCtx is the same as [LDScopedClient.MigrationVariation], but
// accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled.
// The context.Context is used by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/evaluating#go
func (c *LDScopedClient) MigrationVariationCtx(
	ctx gocontext.Context,
	key string,
	defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return c.client.MigrationVariationCtx(ctx, key, c.CurrentContext(), defaultStage)
}

// Identify sends an identify event for the current evaluation context.
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/identify#go
func (c *LDScopedClient) Identify() error {
	return c.client.Identify(c.CurrentContext())
}

// TrackEvent sends a custom event for the current evaluation context.
//
// The eventName parameter is defined by the application and will be shown in
// analytics reports; it normally corresponds to the event name of a metric that
// you have created through the LaunchDarkly dashboard. If you want to associate
// additional data with this event, use [TrackData] or [TrackMetric].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/events#go
func (c *LDScopedClient) TrackEvent(eventName string) error {
	return c.client.TrackEvent(eventName, c.CurrentContext())
}

// TrackEventCtx is the same as [LDScopedClient.TrackEvent], but accepts a
// context.Context.
//
// Cancelling the context.Context will not cause the track operation to be
// cancelled. The context.Context is used by hook implementations refer to
// [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/events#go
func (c *LDScopedClient) TrackEventCtx(
	ctx gocontext.Context,
	eventName string,
) error {
	return c.client.TrackEventCtx(ctx, eventName, c.CurrentContext())
}

// TrackData sends a custom event for the current evaluation context, with custom
// data.
//
// The eventName parameter is defined by the application and will be shown in
// analytics reports; it normally corresponds to the event name of a metric that
// you have created through the LaunchDarkly dashboard.
//
// The data parameter is a value of any JSON type, represented with the
// [ldvalue.Value] type, that will be sent with the event. If no such value is
// needed, use [ldvalue.Null]() (or call [TrackEvent] instead). To send a numeric
// value for experimentation, use [TrackMetric].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/events#go
func (c *LDScopedClient) TrackData(eventName string, data ldvalue.Value) error {
	return c.client.TrackData(eventName, c.CurrentContext(), data)
}

// TrackDataCtx is the same as [LDScopedClient.TrackData], but accepts a
// context.Context.
//
// Cancelling the context.Context will not cause the track operation to be
// cancelled. The context.Context is used by hook implementations refer to
// [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/events#go
func (c *LDScopedClient) TrackDataCtx(
	ctx gocontext.Context,
	eventName string,
	data ldvalue.Value,
) error {
	return c.client.TrackDataCtx(ctx, eventName, c.CurrentContext(), data)
}

// TrackMetric sends a custom event for the current evaluation context, with a
// numeric value.
//
// The eventName parameter is defined by the application and will be shown in
// analytics reports; it normally corresponds to the event name of a metric that
// you have created through the LaunchDarkly dashboard.
//
// The data parameter is a value of any JSON type, represented with the
// [ldvalue.Value] type, that will be sent with the event. If no such value is
// needed, use [ldvalue.Null]().
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/events#go
func (c *LDScopedClient) TrackMetric(eventName string, metricValue float64, data ldvalue.Value) error {
	return c.client.TrackMetric(eventName, c.CurrentContext(), metricValue, data)
}

// TrackMetricCtx is the same as [LDScopedClient.TrackMetric], but accepts a
// context.Context.
//
// Cancelling the context.Context will not cause the track operation to be
// cancelled. The context.Context is used by hook implementations refer to
// [ldhooks.Hook].
//
// For more information, see the Reference Guide:
// https://docs.launchdarkly.com/sdk/features/events#go
func (c *LDScopedClient) TrackMetricCtx(
	ctx gocontext.Context,
	eventName string,
	metricValue float64,
	data ldvalue.Value,
) error {
	return c.client.TrackMetricCtx(ctx, eventName, c.CurrentContext(), metricValue, data)
}

// TrackMigrationOp reports a migration operation event.
func (c *LDScopedClient) TrackMigrationOp(event ldevents.MigrationOpEventData) error {
	return c.client.TrackMigrationOp(event)
}

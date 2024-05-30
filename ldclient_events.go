package ldclient

import (
	gocontext "context"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	ldeval "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces/flagstate"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// This file contains internal support code for LDClient's interactions with the analytics event pipeline.
//
// General implementation notes:
//
// Under normal circumstances, an analytics event is generated whenever 1. a flag is evaluated explicitly
// with a Variation method, 2. a flag is evaluated indirectly as a prerequisite, or 3. the application
// explicitly generates an event by calling Identify or Track. This event is submitted to the configured
// EventProcessor's SendEvent method; the EventProcessor then does any necessary processing and eventually
// delivers the event data to LaunchDarkly, either as a full event or in summary data. The implementation
// of that logic is all in go-sdk-events (since it can be used outside of the SDK, as in ld-relay).
//
// When events are disabled, the EventProcessor is a null implementation that does nothing. It is safe to
// use that object, but LDClient still refrains from doing so if it knows events are disabled so that we
// can avoid a little bit of computational overhead. That's the reason for the IsNullEventProcessorFactory
// method.

type nullEventProcessorFactoryDescription interface {
	IsNullEventProcessorFactory() bool
}

func isNullEventProcessorFactory(f subsystems.ComponentConfigurer[ldevents.EventProcessor]) bool {
	if nf, ok := f.(nullEventProcessorFactoryDescription); ok {
		return nf.IsNullEventProcessorFactory()
	}
	return false
}

func getEventProcessorFactory(config Config) subsystems.ComponentConfigurer[ldevents.EventProcessor] {
	if config.Offline {
		return ldcomponents.NoEvents()
	}
	if config.Events == nil {
		return ldcomponents.SendEvents()
	}
	return config.Events
}

// This struct is used during evaluations to keep track of the event generation strategy we are using
// (with or without evaluation reasons). It captures all of the relevant state so that we do not need to
// create any more stateful objects, such as closures, to generate events during an evaluation. See
// CONTRIBUTING.md for performance issues with closures.
type eventsScope struct {
	disabled                  bool
	factory                   ldevents.EventFactory
	prerequisiteEventRecorder ldeval.PrerequisiteFlagEventRecorder
}

func newDisabledEventsScope() eventsScope {
	return eventsScope{disabled: true}
}

func newEventsScope(client *LDClient, withReasons bool) eventsScope {
	factory := ldevents.NewEventFactory(withReasons, nil)
	return eventsScope{
		factory: factory,
		prerequisiteEventRecorder: func(params ldeval.PrerequisiteFlagEvent) {
			client.eventProcessor.RecordEvaluation(factory.NewEvaluationData(
				ldevents.FlagEventProperties{
					Key:                  params.PrerequisiteFlag.Key,
					Version:              params.PrerequisiteFlag.Version,
					RequireFullEvent:     params.PrerequisiteFlag.TrackEvents,
					DebugEventsUntilDate: params.PrerequisiteFlag.DebugEventsUntilDate,
				},
				ldevents.Context(params.Context),
				params.PrerequisiteResult.Detail,
				params.PrerequisiteResult.IsExperiment,
				ldvalue.Null(),
				params.TargetFlagKey,
				params.PrerequisiteFlag.SamplingRatio,
				params.ExcludeFromSummaries,
			))
		},
	}
}

// This implementation of interfaces.LDClientInterface delegates all client operations to the
// underlying LDClient, but suppresses the generation of analytics events.
type clientEventsDisabledDecorator struct {
	client *LDClient
	scope  eventsScope
}

func newClientEventsDisabledDecorator(client *LDClient) interfaces.LDClientInterface {
	return &clientEventsDisabledDecorator{client: client, scope: newDisabledEventsScope()}
}

func (c *clientEventsDisabledDecorator) BoolVariation(
	key string,
	context ldcontext.Context,
	defaultVal bool,
) (bool, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Bool(defaultVal),
		true, c.scope, boolVarFuncName)
	return detail.Value.BoolValue(), err
}

func (c *clientEventsDisabledDecorator) BoolVariationDetail(key string, context ldcontext.Context, defaultVal bool) (
	bool, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Bool(defaultVal),
		true, c.scope, boolVarDetailFuncName)
	return detail.Value.BoolValue(), detail, err
}

func (c *clientEventsDisabledDecorator) BoolVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal bool,
) (bool, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.Bool(defaultVal),
		true, c.scope, boolVarExFuncName)
	return detail.Value.BoolValue(), err
}

func (c *clientEventsDisabledDecorator) BoolVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal bool,
) (
	bool, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.Bool(defaultVal),
		true, c.scope, boolVarDetailExFuncName)
	return detail.Value.BoolValue(), detail, err
}

func (c *clientEventsDisabledDecorator) IntVariation(
	key string,
	context ldcontext.Context,
	defaultVal int,
) (int, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Int(defaultVal),
		true, c.scope, intVarFuncName)
	return detail.Value.IntValue(), err
}

func (c *clientEventsDisabledDecorator) IntVariationDetail(key string, context ldcontext.Context, defaultVal int) (
	int, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Int(defaultVal),
		true, c.scope, intVarDetailFuncName)
	return detail.Value.IntValue(), detail, err
}

func (c *clientEventsDisabledDecorator) IntVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal int,
) (int, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.Int(defaultVal),
		true, c.scope, intVarExFuncName)
	return detail.Value.IntValue(), err
}

func (c *clientEventsDisabledDecorator) IntVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal int,
) (
	int, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.Int(defaultVal),
		true, c.scope, intVarDetailExFuncName)
	return detail.Value.IntValue(), detail, err
}

func (c *clientEventsDisabledDecorator) Float64Variation(key string, context ldcontext.Context, defaultVal float64) (
	float64, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Float64(defaultVal),
		true, c.scope, floatVarFuncName)
	return detail.Value.Float64Value(), err
}

func (c *clientEventsDisabledDecorator) Float64VariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal float64,
) (
	float64, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Float64(defaultVal),
		true, c.scope, floatVarDetailFuncName)
	return detail.Value.Float64Value(), detail, err
}

func (c *clientEventsDisabledDecorator) Float64VariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal float64,
) (
	float64, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.Float64(defaultVal),
		true, c.scope, floatVarExFuncName)
	return detail.Value.Float64Value(), err
}

func (c *clientEventsDisabledDecorator) Float64VariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal float64,
) (
	float64, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.Float64(defaultVal),
		true, c.scope, floatVarDetailExFuncName)
	return detail.Value.Float64Value(), detail, err
}

func (c *clientEventsDisabledDecorator) StringVariation(key string, context ldcontext.Context, defaultVal string) (
	string, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.String(defaultVal),
		true, c.scope, stringVarExFuncName)
	return detail.Value.StringValue(), err
}

func (c *clientEventsDisabledDecorator) StringVariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal string,
) (
	string, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.String(defaultVal),
		true, c.scope, stringVarDetailFuncName)
	return detail.Value.StringValue(), detail, err
}

func (c *clientEventsDisabledDecorator) StringVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal string,
) (
	string, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.String(defaultVal), true, c.scope,
		stringVarExFuncName)
	return detail.Value.StringValue(), err
}

func (c *clientEventsDisabledDecorator) StringVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal string,
) (
	string, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(ctx, key, context, ldvalue.String(defaultVal), true, c.scope,
		stringVarDetailExFuncName)
	return detail.Value.StringValue(), detail, err
}

func (c *clientEventsDisabledDecorator) MigrationVariation(
	key string, context ldcontext.Context, defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return c.client.migrationVariation(gocontext.TODO(), key, context, defaultStage, c.scope, migrationVarFuncName)
}

func (c *clientEventsDisabledDecorator) MigrationVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return c.client.migrationVariation(ctx, key, context, defaultStage, c.scope, migrationVarExFuncName)
}

func (c *clientEventsDisabledDecorator) JSONVariation(key string, context ldcontext.Context, defaultVal ldvalue.Value) (
	ldvalue.Value, error) {
	detail, _, err := c.client.variationWithHooks(
		gocontext.TODO(),
		key,
		context,
		defaultVal,
		true,
		c.scope,
		jsonVarFuncName,
	)
	return detail.Value, err
}

func (c *clientEventsDisabledDecorator) JSONVariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
) (
	ldvalue.Value, ldreason.EvaluationDetail, error) {
	detail, _, err := c.client.variationWithHooks(
		gocontext.TODO(),
		key,
		context,
		defaultVal,
		true,
		c.scope,
		jsonVarDetailFuncName,
	)
	return detail.Value, detail, err
}

func (c *clientEventsDisabledDecorator) AllFlagsState(
	context ldcontext.Context,
	options ...flagstate.Option,
) flagstate.AllFlags {
	// Currently AllFlagsState never generates events anyway, so nothing is different here
	return c.client.AllFlagsState(context, options...)
}

func (c *clientEventsDisabledDecorator) Identify(context ldcontext.Context) error {
	return nil
}

func (c *clientEventsDisabledDecorator) TrackEvent(eventName string, context ldcontext.Context) error {
	return nil
}

func (c *clientEventsDisabledDecorator) TrackData(
	eventName string,
	context ldcontext.Context,
	data ldvalue.Value,
) error {
	return nil
}

func (c *clientEventsDisabledDecorator) TrackMetric(eventName string, context ldcontext.Context, metricValue float64,
	data ldvalue.Value) error {
	return nil
}

func (c *clientEventsDisabledDecorator) TrackMigrationOp(event ldevents.MigrationOpEventData) error {
	return nil
}

func (c *clientEventsDisabledDecorator) WithEventsDisabled(disabled bool) interfaces.LDClientInterface {
	if disabled {
		return c
	}
	return c.client
}

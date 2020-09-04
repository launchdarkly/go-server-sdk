package ldclient

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/flagstate"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
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

func isNullEventProcessorFactory(f interfaces.EventProcessorFactory) bool {
	if nf, ok := f.(nullEventProcessorFactoryDescription); ok {
		return nf.IsNullEventProcessorFactory()
	}
	return false
}

func getEventProcessorFactory(config Config) interfaces.EventProcessorFactory {
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
			event := factory.NewEvalEvent(
				params.PrerequisiteFlag,
				ldevents.User(params.User),
				params.PrerequisiteResult,
				ldvalue.Null(),
				params.TargetFlagKey,
			)
			client.eventProcessor.RecordFeatureRequestEvent(event)
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

func (c *clientEventsDisabledDecorator) BoolVariation(key string, user lduser.User, defaultVal bool) (bool, error) {
	detail, err := c.client.variation(key, user, ldvalue.Bool(defaultVal), true, c.scope)
	return detail.Value.BoolValue(), err
}

func (c *clientEventsDisabledDecorator) BoolVariationDetail(key string, user lduser.User, defaultVal bool) (
	bool, ldreason.EvaluationDetail, error) {
	detail, err := c.client.variation(key, user, ldvalue.Bool(defaultVal), true, c.scope)
	return detail.Value.BoolValue(), detail, err
}

func (c *clientEventsDisabledDecorator) IntVariation(key string, user lduser.User, defaultVal int) (int, error) {
	detail, err := c.client.variation(key, user, ldvalue.Int(defaultVal), true, c.scope)
	return detail.Value.IntValue(), err
}

func (c *clientEventsDisabledDecorator) IntVariationDetail(key string, user lduser.User, defaultVal int) (
	int, ldreason.EvaluationDetail, error) {
	detail, err := c.client.variation(key, user, ldvalue.Int(defaultVal), true, c.scope)
	return detail.Value.IntValue(), detail, err
}

func (c *clientEventsDisabledDecorator) Float64Variation(key string, user lduser.User, defaultVal float64) (
	float64, error) {
	detail, err := c.client.variation(key, user, ldvalue.Float64(defaultVal), true, c.scope)
	return detail.Value.Float64Value(), err
}

func (c *clientEventsDisabledDecorator) Float64VariationDetail(key string, user lduser.User, defaultVal float64) (
	float64, ldreason.EvaluationDetail, error) {
	detail, err := c.client.variation(key, user, ldvalue.Float64(defaultVal), true, c.scope)
	return detail.Value.Float64Value(), detail, err
}

func (c *clientEventsDisabledDecorator) StringVariation(key string, user lduser.User, defaultVal string) (
	string, error) {
	detail, err := c.client.variation(key, user, ldvalue.String(defaultVal), true, c.scope)
	return detail.Value.StringValue(), err
}

func (c *clientEventsDisabledDecorator) StringVariationDetail(key string, user lduser.User, defaultVal string) (
	string, ldreason.EvaluationDetail, error) {
	detail, err := c.client.variation(key, user, ldvalue.String(defaultVal), true, c.scope)
	return detail.Value.StringValue(), detail, err
}

func (c *clientEventsDisabledDecorator) JSONVariation(key string, user lduser.User, defaultVal ldvalue.Value) (
	ldvalue.Value, error) {
	detail, err := c.client.variation(key, user, defaultVal, true, c.scope)
	return detail.Value, err
}

func (c *clientEventsDisabledDecorator) JSONVariationDetail(key string, user lduser.User, defaultVal ldvalue.Value) (
	ldvalue.Value, ldreason.EvaluationDetail, error) {
	detail, err := c.client.variation(key, user, defaultVal, true, c.scope)
	return detail.Value, detail, err
}

func (c *clientEventsDisabledDecorator) AllFlagsState(
	user lduser.User,
	options ...flagstate.Option,
) flagstate.AllFlags {
	// Currently AllFlagsState never generates events anyway, so nothing is different here
	return c.client.AllFlagsState(user, options...)
}

func (c *clientEventsDisabledDecorator) Identify(user lduser.User) error {
	return nil
}

func (c *clientEventsDisabledDecorator) TrackEvent(eventName string, user lduser.User) error {
	return nil
}

func (c *clientEventsDisabledDecorator) TrackData(eventName string, user lduser.User, data ldvalue.Value) error {
	return nil
}

func (c *clientEventsDisabledDecorator) TrackMetric(eventName string, user lduser.User, metricValue float64,
	data ldvalue.Value) error {
	return nil
}

func (c *clientEventsDisabledDecorator) WithEventsDisabled(disabled bool) interfaces.LDClientInterface {
	if disabled {
		return c
	}
	return c.client
}

package sharedtest

import (
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
)

// SingleEventProcessorFactory is a test implementation of EventProcessorFactory that always returns the same
// pre-existing instance.
type SingleEventProcessorFactory struct {
	Instance ldevents.EventProcessor
}

func (f SingleEventProcessorFactory) CreateEventProcessor( //nolint:revive
	context interfaces.ClientContext,
) (ldevents.EventProcessor, error) {
	return f.Instance, nil
}

// CapturingEventProcessor is a test implementation of EventProcessor that accumulates all events.
type CapturingEventProcessor struct {
	Events []interface{}
}

func (c *CapturingEventProcessor) RecordFeatureRequestEvent(e ldevents.FeatureRequestEvent) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordIdentifyEvent(e ldevents.IdentifyEvent) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordCustomEvent(e ldevents.CustomEvent) { //nolint:revive
	c.Events = append(c.Events, e)
}

// RecordAliasEvent is temporarily retained here even though alias events have been removed from the rest of
// go-server-sdk, because we are not yet using the new version of ldevents, so the ldevents EventProcessor
// interface that CapturingEventProcessor is implementing still needs to have this method.
func (c *CapturingEventProcessor) RecordAliasEvent(e ldevents.AliasEvent) {
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) Flush() {} //nolint:revive

func (c *CapturingEventProcessor) Close() error { //nolint:revive
	return nil
}

package sharedtest

import (
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// SingleEventProcessorFactory is a test implementation of EventProcessorFactory that always returns the same
// pre-existing instance.
type SingleEventProcessorFactory struct {
	Instance ldevents.EventProcessor
}

func (f SingleEventProcessorFactory) CreateEventProcessor( //nolint:golint
	context interfaces.ClientContext,
) (ldevents.EventProcessor, error) {
	return f.Instance, nil
}

// CapturingEventProcessor is a test implementation of EventProcessor that accumulates all events.
type CapturingEventProcessor struct {
	Events []ldevents.Event
}

func (c *CapturingEventProcessor) RecordFeatureRequestEvent(e ldevents.FeatureRequestEvent) { //nolint:golint
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordIdentifyEvent(e ldevents.IdentifyEvent) { //nolint:golint
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordCustomEvent(e ldevents.CustomEvent) { //nolint:golint
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) Flush() {} //nolint:golint

func (c *CapturingEventProcessor) Close() error { //nolint:golint
	return nil
}

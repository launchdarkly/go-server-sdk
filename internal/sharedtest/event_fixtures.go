package sharedtest

import (
	"encoding/json"

	ldevents "github.com/launchdarkly/go-sdk-events/v2"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// SingleEventProcessorFactory is a test implementation of EventProcessorFactory that always returns the same
// pre-existing instance.
type SingleEventProcessorFactory struct {
	Instance ldevents.EventProcessor
}

func (f SingleEventProcessorFactory) CreateEventProcessor( //nolint:revive
	context subsystems.ClientContext,
) (ldevents.EventProcessor, error) {
	return f.Instance, nil
}

// CapturingEventProcessor is a test implementation of EventProcessor that accumulates all events.
type CapturingEventProcessor struct {
	Events []interface{}
}

func (c *CapturingEventProcessor) RecordEvaluation(e ldevents.EvaluationData) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordIdentifyEvent(e ldevents.IdentifyEventData) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordCustomEvent(e ldevents.CustomEventData) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordRawEvent(e json.RawMessage) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) Flush() {} //nolint:revive

func (c *CapturingEventProcessor) Close() error { //nolint:revive
	return nil
}

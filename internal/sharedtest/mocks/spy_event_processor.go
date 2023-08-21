package mocks

import (
	"encoding/json"
	"time"

	ldevents "github.com/launchdarkly/go-sdk-events/v2"
)

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

func (c *CapturingEventProcessor) RecordMigrationOpEvent(e ldevents.MigrationOpEventData) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) RecordRawEvent(e json.RawMessage) { //nolint:revive
	c.Events = append(c.Events, e)
}

func (c *CapturingEventProcessor) Flush() {} //nolint:revive

func (c *CapturingEventProcessor) FlushBlocking(time.Duration) bool { return true } //nolint:revive

func (c *CapturingEventProcessor) Close() error { //nolint:revive
	return nil
}

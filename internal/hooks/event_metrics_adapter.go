package hooks

import (
	ldevents "github.com/launchdarkly/go-sdk-events/v3"

	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// EventMetricsAdapter forwards event processing metrics from the event processor to hook handlers.
//
// The event processor reports the same facts through several EventMetrics methods. The adapter
// uses only RecordFlush, which carries the complete information for one hook call, including the
// number of events discarded since the previous flush.
type EventMetricsAdapter struct {
	runner *Runner
}

// NewEventMetricsAdapter creates an EventMetricsAdapter that dispatches to the given runner.
func NewEventMetricsAdapter(runner *Runner) *EventMetricsAdapter {
	return &EventMetricsAdapter{runner: runner}
}

// RecordDroppedEvents is a no-op; drops are reported with the next flush through RecordFlush.
func (a *EventMetricsAdapter) RecordDroppedEvents(int) {}

// RecordEventsSent is a no-op; deliveries are reported through RecordFlush.
func (a *EventMetricsAdapter) RecordEventsSent(int) {}

// RecordEventsFailedSend is a no-op; failures are reported through RecordFlush.
func (a *EventMetricsAdapter) RecordEventsFailedSend(int, ldevents.EventSendFailureMetadata) {}

// RecordEventsBytesSent is a no-op; payload sizes are reported through RecordFlush.
func (a *EventMetricsAdapter) RecordEventsBytesSent(int) {}

// RecordPendingEvents is a no-op. Queue depth changes on every event and is not exposed to hooks.
func (a *EventMetricsAdapter) RecordPendingEvents(int) {}

// RecordFlush runs the EventFlushCompleted handlers.
func (a *EventMetricsAdapter) RecordFlush(result ldevents.EventFlushResult) {
	a.runner.RunEventFlushCompleted(ldhooks.NewEventFlushContext(
		result.EventCount,
		result.PayloadBytes,
		result.Success,
		result.StatusCode,
		result.Duration,
		result.DroppedCount,
	))
}

var (
	_ ldevents.EventMetrics = (*EventMetricsAdapter)(nil)
	_ ldevents.FlushMetrics = (*EventMetricsAdapter)(nil)
)

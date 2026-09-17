package hooks

import (
	ldevents "github.com/launchdarkly/go-sdk-events/v3"

	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// EventMetricsAdapter forwards event processing metrics from the event processor to hook handlers.
//
// The event processor reports the same facts through several EventMetrics methods. The adapter
// uses only the two extended methods, RecordFlush and RecordDroppedEventsWithReason, because they
// carry the complete information for one hook call each.
type EventMetricsAdapter struct {
	runner *Runner
}

// NewEventMetricsAdapter creates an EventMetricsAdapter that dispatches to the given runner.
func NewEventMetricsAdapter(runner *Runner) *EventMetricsAdapter {
	return &EventMetricsAdapter{runner: runner}
}

// RecordDroppedEvents is a no-op; drops are reported through RecordDroppedEventsWithReason.
func (a *EventMetricsAdapter) RecordDroppedEvents(int) {}

// RecordEventsSent is a no-op; deliveries are reported through RecordFlush.
func (a *EventMetricsAdapter) RecordEventsSent(int) {}

// RecordEventsFailedSend is a no-op; failures are reported through RecordFlush.
func (a *EventMetricsAdapter) RecordEventsFailedSend(int, ldevents.EventSendFailureMetadata) {}

// RecordEventsBytesSent is a no-op; payload sizes are reported through RecordFlush.
func (a *EventMetricsAdapter) RecordEventsBytesSent(int) {}

// RecordPendingEvents is a no-op. Queue depth changes on every event and is not exposed to hooks.
func (a *EventMetricsAdapter) RecordPendingEvents(int) {}

// RecordFlush runs the AfterEventFlush handlers.
func (a *EventMetricsAdapter) RecordFlush(result ldevents.EventFlushResult) {
	a.runner.RunAfterEventFlush(ldhooks.NewEventFlushContext(
		result.EventCount,
		result.PayloadBytes,
		result.Success,
		result.StatusCode,
		result.Duration,
	))
}

// RecordDroppedEventsWithReason runs the EventsDropped handlers.
func (a *EventMetricsAdapter) RecordDroppedEventsWithReason(count int, reason ldevents.DroppedEventsReason) {
	var hookReason ldhooks.EventsDroppedReason
	switch reason {
	case ldevents.DroppedEventsReasonBackpressure:
		hookReason = ldhooks.EventsDroppedReasonBackpressure
	default:
		hookReason = ldhooks.EventsDroppedReasonCapacity
	}
	a.runner.RunEventsDropped(ldhooks.NewEventsDroppedContext(count, hookReason))
}

var (
	_ ldevents.EventMetrics = (*EventMetricsAdapter)(nil)
	_ ldevents.FlushMetrics = (*EventMetricsAdapter)(nil)
	_ ldevents.DropMetrics  = (*EventMetricsAdapter)(nil)
)

package ldhooks

import "time"

// EventFlushContext contains the information passed to the AfterEventFlush handler.
//
// The SDK creates a new EventFlushContext for each attempt to deliver a batch of analytics events.
type EventFlushContext struct {
	eventCount   int
	payloadBytes int
	success      bool
	statusCode   int
	duration     time.Duration
}

// NewEventFlushContext creates an EventFlushContext. This is called by the SDK.
func NewEventFlushContext(
	eventCount int,
	payloadBytes int,
	success bool,
	statusCode int,
	duration time.Duration,
) EventFlushContext {
	return EventFlushContext{
		eventCount:   eventCount,
		payloadBytes: payloadBytes,
		success:      success,
		statusCode:   statusCode,
		duration:     duration,
	}
}

// EventCount returns the number of events in the batch. A summary event counts as one event.
func (c EventFlushContext) EventCount() int {
	return c.eventCount
}

// PayloadBytes returns the size of the serialized payload before compression.
func (c EventFlushContext) PayloadBytes() int {
	return c.payloadBytes
}

// Success returns true if the events service accepted the batch.
//
// When Success is false, the events in the batch are lost. The SDK does not retry the batch again.
func (c EventFlushContext) Success() bool {
	return c.success
}

// StatusCode returns the HTTP status code of the last response, or 0 if no response was received.
func (c EventFlushContext) StatusCode() int {
	return c.statusCode
}

// Duration returns the total time spent on the delivery attempt, including any retry.
func (c EventFlushContext) Duration() time.Duration {
	return c.duration
}

// EventsDroppedReason identifies why the SDK discarded events.
type EventsDroppedReason string

const (
	// EventsDroppedReasonCapacity means the event buffer reached its configured capacity before a flush.
	EventsDroppedReasonCapacity EventsDroppedReason = "capacity"

	// EventsDroppedReasonBackpressure means the application produced events faster than the SDK could
	// accept them.
	EventsDroppedReasonBackpressure EventsDroppedReason = "backpressure"
)

// EventsDroppedContext contains the information passed to the EventsDropped handler.
type EventsDroppedContext struct {
	count  int
	reason EventsDroppedReason
}

// NewEventsDroppedContext creates an EventsDroppedContext. This is called by the SDK.
func NewEventsDroppedContext(count int, reason EventsDroppedReason) EventsDroppedContext {
	return EventsDroppedContext{count: count, reason: reason}
}

// Count returns the number of events that were discarded.
func (c EventsDroppedContext) Count() int {
	return c.count
}

// Reason returns why the events were discarded.
func (c EventsDroppedContext) Reason() EventsDroppedReason {
	return c.reason
}

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
	droppedCount int
}

// NewEventFlushContext creates an EventFlushContext. This is called by the SDK.
func NewEventFlushContext(
	eventCount int,
	payloadBytes int,
	success bool,
	statusCode int,
	duration time.Duration,
	droppedCount int,
) EventFlushContext {
	return EventFlushContext{
		eventCount:   eventCount,
		payloadBytes: payloadBytes,
		success:      success,
		statusCode:   statusCode,
		duration:     duration,
		droppedCount: droppedCount,
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

// DroppedCount returns the number of events the SDK discarded before delivery since the previous
// flush attempt, because the event buffer was full.
func (c EventFlushContext) DroppedCount() int {
	return c.droppedCount
}

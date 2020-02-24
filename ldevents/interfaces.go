package ldevents

// EventProcessor defines the interface for dispatching analytics events.
type EventProcessor interface {
	// SendEvent records an event asynchronously.
	SendEvent(Event)
	// Flush specifies that any buffered events should be sent as soon as possible, rather than waiting
	// for the next flush interval. This method is asynchronous, so events still may not be sent
	// until a later time.
	Flush()
	// Close shuts down all event processor activity, after first ensuring that all events have been
	// delivered. Subsequent calls to SendEvent() or Flush() will be ignored.
	Close() error
}

type EventSender interface {
	SendEventData(kind EventDataKind, data []byte, eventCount int) EventSenderResult
}

type EventDataKind string

const (
	AnalyticsEventDataKind  EventDataKind = "analytics"
	DiagnosticEventDataKind EventDataKind = "diagnostic"
)

type EventSenderResult struct {
	Success        bool
	MustShutDown   bool
	TimeFromServer uint64
}

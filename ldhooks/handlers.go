package ldhooks

import "context"

// Handlers are hook methods that the SDK runs for internal operations, in contrast to stages,
// which run for operations that the application starts. Examples of internal operations are
// background event delivery and data source status changes.
//
// Handlers are optional. A hook opts in to a handler by implementing the corresponding interface
// in addition to Hook. The SDK checks for these interfaces once, when the client is created, so a
// hook that does not implement them adds no cost to the operation.
//
// The SDK runs handlers in the order of hook registration. An error returned by a handler is logged
// and does not affect the SDK operation.

// DataSourceStatusHandler is an optional hook interface for observing data source status changes.
type DataSourceStatusHandler interface {
	// DataSourceStatusChanged is called when the data source changes state, or when the data source
	// reports a new error while remaining in the same state.
	//
	// The SDK calls this method in-band from the data source. Implementations should return quickly
	// and must not call methods on the client that wait for a data source status.
	DataSourceStatusChanged(ctx context.Context, statusContext DataSourceStatusContext) error
}

// EventFlushHandler is an optional hook interface for observing analytics event delivery.
type EventFlushHandler interface {
	// AfterEventFlush is called after each attempt to deliver a batch of analytics events to
	// LaunchDarkly, whether the attempt succeeded or failed.
	AfterEventFlush(ctx context.Context, flushContext EventFlushContext) error
}

// EventsDroppedHandler is an optional hook interface for observing discarded analytics events.
type EventsDroppedHandler interface {
	// EventsDropped is called when the SDK discards analytics events before delivery.
	//
	// The SDK may call this method frequently when the application produces events faster than
	// the SDK can deliver them. Implementations should return quickly.
	EventsDropped(ctx context.Context, droppedContext EventsDroppedContext) error
}

// InitializerHandler is an optional hook interface for observing data initializer attempts.
type InitializerHandler interface {
	// InitializerCompleted is called after each attempt to obtain initial data from one initializer.
	InitializerCompleted(ctx context.Context, initializerContext InitializerContext) error
}

// SynchronizerHandler is an optional hook interface for observing which synchronizer is active.
type SynchronizerHandler interface {
	// SynchronizerChanged is called when a synchronizer starts, and when no synchronizers remain.
	SynchronizerChanged(ctx context.Context, changeContext SynchronizerChangeContext) error
}

// InitializationHandler is an optional hook interface for observing the end of initialization.
type InitializationHandler interface {
	// InitializationCompleted is called once, when the SDK first has data or has given up obtaining it.
	InitializationCompleted(ctx context.Context, initializationContext InitializationContext) error
}

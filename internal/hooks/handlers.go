package hooks

import (
	gocontext "context"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// namedHandler pairs an optional handler implementation with the name of the hook that provides it,
// for use in error messages.
type namedHandler[H any] struct {
	name    string
	handler H
}

// collectHandlers returns the hooks that implement the optional handler interface H, in registration order.
func collectHandlers[H any](hooks []ldhooks.Hook) []namedHandler[H] {
	var result []namedHandler[H]
	for _, hook := range hooks {
		if handler, ok := hook.(H); ok {
			result = append(result, namedHandler[H]{name: hook.Metadata().Name(), handler: handler})
		}
	}
	return result
}

// HasDataSourceHandlers returns true if any registered hook implements a data source handler:
// DataSourceStatusHandler, InitializerHandler, SynchronizerHandler, or InitializationHandler.
func (h *Runner) HasDataSourceHandlers() bool {
	return len(h.statusHandlers) > 0 || len(h.initializerHandlers) > 0 ||
		len(h.synchronizerHandlers) > 0 || len(h.initializationHandlers) > 0
}

// HasEventDeliveryHandlers returns true if any registered hook implements EventFlushHandler.
func (h *Runner) HasEventDeliveryHandlers() bool {
	return len(h.flushHandlers) > 0
}

// OnDataSourceStatusChanged runs the DataSourceStatusChanged handler of each hook that implements it.
func (h *Runner) OnDataSourceStatusChanged(previous, current interfaces.DataSourceStatus) {
	if len(h.statusHandlers) == 0 {
		return
	}
	statusContext := ldhooks.NewDataSourceStatusContext(previous, current)
	for _, nh := range h.statusHandlers {
		if err := nh.handler.DataSourceStatusChanged(gocontext.Background(), statusContext); err != nil {
			h.loggers.Errorf(
				"During a data source status change, an error was encountered in \"DataSourceStatusChanged\" of the \"%s\" hook: %s",
				nh.name,
				err.Error(),
			)
		}
	}
}

// OnInitializerCompleted runs the InitializerCompleted handler of each hook that implements it.
func (h *Runner) OnInitializerCompleted(initializerContext ldhooks.InitializerContext) {
	for _, nh := range h.initializerHandlers {
		if err := nh.handler.InitializerCompleted(gocontext.Background(), initializerContext); err != nil {
			h.loggers.Errorf(
				"During initialization, an error was encountered in \"InitializerCompleted\" of the \"%s\" hook: %s",
				nh.name,
				err.Error(),
			)
		}
	}
}

// OnSynchronizerChanged runs the SynchronizerChanged handler of each hook that implements it.
func (h *Runner) OnSynchronizerChanged(changeContext ldhooks.SynchronizerChangeContext) {
	for _, nh := range h.synchronizerHandlers {
		if err := nh.handler.SynchronizerChanged(gocontext.Background(), changeContext); err != nil {
			h.loggers.Errorf(
				"During a synchronizer change, an error was encountered in \"SynchronizerChanged\" of the \"%s\" hook: %s",
				nh.name,
				err.Error(),
			)
		}
	}
}

// OnInitializationCompleted runs the InitializationCompleted handler of each hook that implements it.
func (h *Runner) OnInitializationCompleted(initializationContext ldhooks.InitializationContext) {
	for _, nh := range h.initializationHandlers {
		if err := nh.handler.InitializationCompleted(gocontext.Background(), initializationContext); err != nil {
			h.loggers.Errorf(
				"During initialization, an error was encountered in \"InitializationCompleted\" of the \"%s\" hook: %s",
				nh.name,
				err.Error(),
			)
		}
	}
}

// RunEventFlushCompleted runs the EventFlushCompleted handler of each hook that implements it.
func (h *Runner) RunEventFlushCompleted(flushContext ldhooks.EventFlushContext) {
	for _, nh := range h.flushHandlers {
		if err := nh.handler.EventFlushCompleted(gocontext.Background(), flushContext); err != nil {
			h.loggers.Errorf(
				"During event delivery, an error was encountered in \"EventFlushCompleted\" of the \"%s\" hook: %s",
				nh.name,
				err.Error(),
			)
		}
	}
}

// Ensure that Runner conforms to the internal observer interface.
var _ internal.DataSourceStatusObserver = (*Runner)(nil)

package hooks

import (
	gocontext "context"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// TrackExecution represents the state of a running series of track stages.
type TrackExecution struct {
	hooks   []ldhooks.Hook
	context ldhooks.TrackSeriesContext
	loggers *ldlog.Loggers
}

// AfterTrack executes the AfterTrack stage of registered hooks.
func (t *TrackExecution) AfterTrack(ctx gocontext.Context) {
	iterator := newIterator(true, t.hooks)
	for iterator.hasNext() {
		_, hook := iterator.getNext()
		err := hook.AfterTrack(ctx, t.context)
		if err != nil {
			t.loggers.Errorf(
				"During tracking of event \"%s\", an error was encountered in \"AfterTrack\" of the \"%s\" hook: %s",
				t.context.Key(),
				hook.Metadata().Name(),
				err.Error(),
			)
			continue
		}
	}
}

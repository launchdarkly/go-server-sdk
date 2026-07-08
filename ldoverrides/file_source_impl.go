package ldoverrides

import (
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/filedata"
	"github.com/launchdarkly/go-server-sdk/v7/ldfilewatch"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

type fileOverrideSource struct {
	paths                 []string
	duplicateKeysHandling filedata.DuplicateKeysHandling
	watch                 bool
	poll                  bool
	pollInterval          time.Duration
	loggers               ldlog.Loggers

	reloader     *filedata.Reloader
	poller       *filedata.Poller
	closeWatchCh chan struct{}
	closeOnce    sync.Once
}

var _ subsystems.OverrideSource = (*fileOverrideSource)(nil)

// Start relies on the OverrideSource lifecycle: the SDK calls Start at most once, before
// any call to Close, from a single goroutine. The reloader and trigger sources are created
// here rather than at build time because they need the sink.
func (f *fileOverrideSource) Start(sink subsystems.OverrideSink) {
	f.reloader = filedata.NewReloader(filedata.ReloaderConfig{
		Paths:                 f.paths,
		DuplicateKeysHandling: f.duplicateKeysHandling,
		Loggers:               f.loggers,
		Apply: func(merged filedata.MergeResult) {
			sink.SetOverrides([]ldstoretypes.Collection{
				{Kind: ldstoreimpl.Features(), Items: merged.Flags},
				{Kind: ldstoreimpl.Segments(), Items: merged.Segments},
			})
		},
		DebounceDelay: filedata.DefaultDebounceDelay,
		RetryDelay:    filedata.DefaultRetryDelay,
		SkipUnchanged: true,
	})

	// The initial load happens synchronously, so overrides present in the files are in
	// effect by the time the client constructor returns. A failure here is not fatal: the
	// client runs with no overrides, the failure is logged, and the retry (plus any watch
	// or poll signal) recovers once the files are readable.
	f.reloader.ReloadNow()

	if f.watch {
		f.closeWatchCh = make(chan struct{})
		if err := ldfilewatch.WatchFiles(f.paths, f.loggers, f.reloader.Trigger, f.closeWatchCh); err != nil {
			// COVERAGE: constructing a watcher only fails under unusual OS conditions
			f.loggers.Errorf("Unable to watch override files: %s", err)
		}
	}
	if f.poll {
		f.poller = filedata.NewPoller(f.paths, f.pollInterval, f.reloader.Trigger)
	}
}

func (f *fileOverrideSource) Close() error {
	f.closeOnce.Do(func() {
		if f.closeWatchCh != nil {
			close(f.closeWatchCh)
		}
		if f.poller != nil {
			f.poller.Close()
		}
		if f.reloader != nil {
			f.reloader.Close()
		}
	})
	return nil
}

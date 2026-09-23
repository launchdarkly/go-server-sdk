package ldoverrides

import (
	"fmt"
	"strings"
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
	changeDetection       ChangeDetection
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
		SkipMissingPaths:      true,
		Loggers:               f.loggers,
		Apply: func(merged filedata.MergeResult) {
			sink.SetOverrides([]ldstoretypes.Collection{
				{Kind: ldstoreimpl.Features(), Items: merged.Flags},
				{Kind: ldstoreimpl.Segments(), Items: merged.Segments},
			})
			f.logOverridesInEffect(merged)
		},
		DebounceDelay: filedata.DefaultDebounceDelay,
		RetryDelay:    filedata.DefaultRetryDelay,
		SkipUnchanged: true,
	})

	// The initial load happens synchronously, so overrides present in the files are in
	// effect by the time the client constructor returns. A file that does not exist yet
	// contributes no overrides. A file that cannot be read or parsed is not fatal. The
	// client runs with the last good overrides, the failure is logged, and the retry, plus
	// the change signal, recovers once the file is readable.
	f.reloader.ReloadNow()

	switch f.changeDetection {
	case Watching:
		f.closeWatchCh = make(chan struct{})
		if err := ldfilewatch.WatchOptionalFiles(f.paths, f.loggers, f.reloader.Trigger, f.closeWatchCh); err != nil {
			// COVERAGE: constructing a watcher only fails under unusual OS conditions
			f.loggers.Errorf("Unable to watch override files: %s", err)
		}
	case Polling:
		f.poller = filedata.NewPoller(f.paths, f.pollInterval, f.reloader.Trigger)
	}
}

// logOverridesInEffect reports the overrides now in effect and the file each came from. The
// reloader applies a snapshot only when the content changed, so this logs each change once.
func (f *fileOverrideSource) logOverridesInEffect(merged filedata.MergeResult) {
	details := make([]string, 0, len(merged.Files))
	for _, file := range merged.Files {
		switch {
		case !file.Present:
			details = append(details, file.Path+": absent")
		case file.Flags == 0 && file.Segments == 0:
			details = append(details, file.Path+": no entries")
		default:
			details = append(details, file.Path+": "+countsText(file.Flags, file.Segments))
		}
	}
	if len(merged.Flags) == 0 && len(merged.Segments) == 0 {
		f.loggers.Infof("Flag overrides: none in effect (%s)", strings.Join(details, "; "))
		return
	}
	f.loggers.Infof("Flag overrides in effect: %s (%s)",
		countsText(len(merged.Flags), len(merged.Segments)), strings.Join(details, "; "))
}

// countsText formats flag and segment counts, for example "2 flags, 1 segment".
func countsText(flags int, segments int) string {
	parts := make([]string, 0, 2)
	if flags > 0 {
		parts = append(parts, pluralize(flags, "flag"))
	}
	if segments > 0 {
		parts = append(parts, pluralize(segments, "segment"))
	}
	return strings.Join(parts, ", ")
}

func pluralize(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
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

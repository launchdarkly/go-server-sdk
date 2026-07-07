package ldoverrides

import (
	"fmt"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/filedata"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// DuplicateKeysHandling is a parameter type used with FileSourceBuilder.DuplicateKeysHandling.
type DuplicateKeysHandling string

const (
	// DuplicateKeysFail is an option for FileSourceBuilder.DuplicateKeysHandling, meaning that
	// a reload fails (leaving the previously loaded overrides in effect) if the same flag or
	// segment key appears in more than one file.
	DuplicateKeysFail DuplicateKeysHandling = "fail"

	// DuplicateKeysIgnoreAllButFirst is an option for FileSourceBuilder.DuplicateKeysHandling,
	// meaning that when the same key appears in more than one file, only the entry from the
	// earliest-configured file is used.
	DuplicateKeysIgnoreAllButFirst DuplicateKeysHandling = "ignore"
)

const (
	// DefaultPollInterval is the interval at which the file source examines the files for
	// changes when polling is enabled and no interval was specified. Because the source reads
	// local files rather than contacting a service, a short interval keeps an override
	// responsive during an incident at negligible cost.
	DefaultPollInterval = time.Second

	// MinimumPollInterval is the shortest allowed polling interval. A configured interval
	// below this is raised to it. The minimum exists only to prevent a pathological tight
	// loop over the filesystem.
	MinimumPollInterval = time.Second
)

// FileSourceBuilder is a builder created by FileSource. Configure it with the builder
// methods, then pass it to the Overrides method of the data system configuration builder
// in the ldcomponents package.
type FileSourceBuilder struct {
	filePaths             []string
	duplicateKeysHandling DuplicateKeysHandling
	watch                 bool
	poll                  bool
	pollInterval          time.Duration
}

// FileSource returns a builder for a file-based override source, which reads flag and
// segment overrides from one or more local files and reloads them as the files change.
//
// The files use the same document format as the file data sources (ldfiledata and
// ldfiledatav2): a JSON or YAML document with optional "flags", "flagValues", and
// "segments" members. "flagValues" entries are expanded into full flag definitions that
// return the given value for every context. When multiple files are configured, their
// entries are combined; the configured order determines which file wins under the
// duplicate-key handling.
//
// A reload replaces the entire override set, so removing an entry from the files removes
// the override. A file that is missing or cannot be parsed makes that whole reload fail,
// leaving the previously loaded overrides in effect; the source logs the failure, retries
// after a short delay, and recovers on its own once the files are readable again. At
// startup, failing to load simply means the client runs with no overrides.
//
// By default the source watches the files for changes using filesystem notifications; see
// Watch and Poll for environments where notifications are unavailable or unreliable.
func FileSource() *FileSourceBuilder {
	return &FileSourceBuilder{
		duplicateKeysHandling: DuplicateKeysFail,
		watch:                 true,
		pollInterval:          DefaultPollInterval,
	}
}

// FilePaths specifies the files to load overrides from. The order is significant: it
// determines which file wins under the duplicate-key handling when the same key appears in
// more than one file.
func (b *FileSourceBuilder) FilePaths(paths ...string) *FileSourceBuilder {
	b.filePaths = append(b.filePaths, paths...)
	return b
}

// DuplicateKeysHandling specifies how to handle the same key appearing in more than one
// file. If not specified, or set to an unrecognized value, the default is DuplicateKeysFail.
func (b *FileSourceBuilder) DuplicateKeysHandling(handling DuplicateKeysHandling) *FileSourceBuilder {
	b.duplicateKeysHandling = handling
	return b
}

// Watch enables or disables reloading in response to filesystem change notifications. It
// is enabled by default. Disable it on filesystems where notifications do not work (in
// which case enable Poll instead); both may be enabled together, in which case whichever
// signal arrives first causes the reload.
func (b *FileSourceBuilder) Watch(enabled bool) *FileSourceBuilder {
	b.watch = enabled
	return b
}

// Poll enables or disables examining the files for changes on a fixed interval. It is
// disabled by default. Polling is useful where filesystem notifications are unavailable or
// unreliable, such as some network filesystems, container mounts, and directories whose
// contents are swapped via symlinks (as Kubernetes does for mounted ConfigMaps).
func (b *FileSourceBuilder) Poll(enabled bool) *FileSourceBuilder {
	b.poll = enabled
	return b
}

// PollInterval sets the interval used when polling is enabled. The default is
// DefaultPollInterval; an interval below MinimumPollInterval is raised to the minimum.
func (b *FileSourceBuilder) PollInterval(interval time.Duration) *FileSourceBuilder {
	b.pollInterval = interval
	return b
}

// Build is called internally by the SDK.
func (b *FileSourceBuilder) Build(context subsystems.ClientContext) (subsystems.OverrideSource, error) {
	if len(b.filePaths) == 0 {
		return nil, fmt.Errorf("no file paths were specified for the file-based override source")
	}
	paths, err := filedata.AbsFilePaths(b.filePaths)
	if err != nil {
		// COVERAGE: there's no reliable cross-platform way to simulate an invalid path in unit tests
		return nil, err
	}

	loggers := context.GetLogging().Loggers
	loggers.SetPrefix("FileOverrideSource:")

	pollInterval := b.pollInterval
	if b.poll && pollInterval < MinimumPollInterval {
		loggers.Warnf("Poll interval %s is below the minimum; using %s", pollInterval, MinimumPollInterval)
		pollInterval = MinimumPollInterval
	}

	return &fileOverrideSource{
		paths:                 paths,
		duplicateKeysHandling: filedata.DuplicateKeysHandling(b.duplicateKeysHandling),
		watch:                 b.watch,
		poll:                  b.poll,
		pollInterval:          pollInterval,
		loggers:               loggers,
	}, nil
}

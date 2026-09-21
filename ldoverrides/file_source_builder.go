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
	// DuplicateKeysFail is an option for FileSourceBuilder.DuplicateKeysHandling. With this
	// option, a reload fails if the same flag or segment key appears in more than one file.
	// The previously loaded overrides stay in effect.
	DuplicateKeysFail DuplicateKeysHandling = "fail"

	// DuplicateKeysKeepFirst is an option for FileSourceBuilder.DuplicateKeysHandling. With
	// this option, when the same key appears in more than one file, the entry from the first
	// configured file is kept. The others are discarded.
	DuplicateKeysKeepFirst DuplicateKeysHandling = "ignore"
)

// ChangeDetection is a parameter type used with FileSourceBuilder.ChangeDetection. It
// selects how the source learns that a file changed.
type ChangeDetection string

const (
	// Polling is an option for FileSourceBuilder.ChangeDetection. The source examines the
	// files on a fixed interval and reloads when the modification time or the size of a file
	// changes. Polling works on every file system, including network mounts and directories
	// whose contents are swapped through symbolic links, as Kubernetes does for mounted
	// ConfigMaps. It is the default.
	Polling ChangeDetection = "polling"

	// Watching is an option for FileSourceBuilder.ChangeDetection. The source reloads in
	// response to file system change notifications. It reacts faster than polling. It
	// depends on notifications, which some file systems do not deliver reliably.
	Watching ChangeDetection = "watching"
)

const (
	// DefaultPollInterval is the interval at which the file source examines the files for
	// changes in Polling mode when no interval was specified. Because the source reads local
	// files rather than contacting a service, a short interval keeps an override responsive
	// during an incident at negligible cost.
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
	changeDetection       ChangeDetection
	pollInterval          time.Duration
}

// FileSource returns a builder for a file-based override source. The source reads flag and
// segment overrides from one or more local files and reloads them as the files change.
//
// The files use the same document format as the file data sources (ldfiledata and
// ldfiledatav2). Each file is a JSON or YAML document with optional "flags", "flagValues",
// and "segments" members. "flagValues" entries are expanded into full flag definitions that
// return the given value for every context. When multiple files are configured, their
// entries are combined. The configured order determines which file wins under the
// duplicate-key handling.
//
// A reload replaces the entire override set, so removing an entry from the files removes
// the override. A file that is missing or cannot be parsed makes that whole reload fail.
// The previously loaded overrides stay in effect. The source logs the failure, retries after
// a short delay, and recovers on its own once the files are readable again. At startup,
// failing to load simply means the client runs with no overrides.
//
// By default the source polls the files for changes once per second. See ChangeDetection
// and PollInterval.
func FileSource() *FileSourceBuilder {
	return &FileSourceBuilder{
		duplicateKeysHandling: DuplicateKeysFail,
		changeDetection:       Polling,
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

// ChangeDetection selects how the source detects file changes. The default is Polling. The
// two modes are alternatives, so setting one replaces the other.
func (b *FileSourceBuilder) ChangeDetection(mode ChangeDetection) *FileSourceBuilder {
	b.changeDetection = mode
	return b
}

// PollInterval sets the interval between examinations of the files in Polling mode. Watching
// mode ignores it. The default is DefaultPollInterval. An interval below MinimumPollInterval
// is raised to the minimum.
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

	switch b.changeDetection {
	case Polling, Watching:
	default:
		return nil, fmt.Errorf("unrecognized change detection mode %q for the file-based override source",
			string(b.changeDetection))
	}

	loggers := context.GetLogging().Loggers
	loggers.SetPrefix("FileOverrideSource:")

	pollInterval := b.pollInterval
	if b.changeDetection == Polling && pollInterval < MinimumPollInterval {
		loggers.Warnf("Poll interval %s is below the minimum; using %s", pollInterval, MinimumPollInterval)
		pollInterval = MinimumPollInterval
	}

	return &fileOverrideSource{
		paths:                 paths,
		duplicateKeysHandling: filedata.DuplicateKeysHandling(b.duplicateKeysHandling),
		changeDetection:       b.changeDetection,
		pollInterval:          pollInterval,
		loggers:               loggers,
	}, nil
}

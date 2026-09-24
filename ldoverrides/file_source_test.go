package ldoverrides

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeout = 10 * time.Second

// capturingSink records every override snapshot it receives.
type capturingSink struct {
	mu        sync.Mutex
	snapshots chan []ldstoretypes.Collection
}

func newCapturingSink() *capturingSink {
	return &capturingSink{snapshots: make(chan []ldstoretypes.Collection, 100)}
}

func (c *capturingSink) SetOverrides(data []ldstoretypes.Collection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshots <- data
}

func (c *capturingSink) requireSnapshot(t *testing.T) []ldstoretypes.Collection {
	t.Helper()
	select {
	case snapshot := <-c.snapshots:
		return snapshot
	case <-time.After(testTimeout):
		require.FailNow(t, "timed out waiting for an override snapshot")
		return nil
	}
}

func (c *capturingSink) requireNoSnapshot(t *testing.T, duration time.Duration) {
	t.Helper()
	select {
	case <-c.snapshots:
		require.FailNow(t, "received an unexpected override snapshot")
	case <-time.After(duration):
	}
}

// flagsByKey extracts the flag entities from a snapshot.
func flagsByKey(t *testing.T, snapshot []ldstoretypes.Collection) map[string]*ldmodel.FeatureFlag {
	t.Helper()
	flags := map[string]*ldmodel.FeatureFlag{}
	for _, coll := range snapshot {
		if coll.Kind.GetName() != "features" {
			continue
		}
		for _, item := range coll.Items {
			flag, ok := item.Item.Item.(*ldmodel.FeatureFlag)
			require.True(t, ok)
			flags[item.Key] = flag
		}
	}
	return flags
}

func buildFileSource(t *testing.T, configure func(*FileSourceBuilder)) (subsystems.OverrideSource, *capturingSink) {
	t.Helper()
	builder := FileSource()
	configure(builder)
	source, err := builder.Build(sharedtest.BasicClientContext())
	require.NoError(t, err)
	sink := newCapturingSink()
	source.Start(sink)
	t.Cleanup(func() { _ = source.Close() })
	return source, sink
}

func buildFileSourceWithLog(
	t *testing.T,
	configure func(*FileSourceBuilder),
) (subsystems.OverrideSource, *capturingSink, *ldlogtest.MockLog) {
	t.Helper()
	builder := FileSource()
	configure(builder)
	mockLog := ldlogtest.NewMockLog()
	context := sharedtest.NewTestContext("", nil, &subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
	source, err := builder.Build(context)
	require.NoError(t, err)
	sink := newCapturingSink()
	source.Start(sink)
	t.Cleanup(func() { _ = source.Close() })
	return source, sink, mockLog
}

// requireInfoLine waits for an Info log line that contains every substring. The source logs
// after it hands the snapshot to the sink, so a test that has just received a snapshot may
// run ahead of the log line.
func requireInfoLine(t *testing.T, mockLog *ldlogtest.MockLog, substrings ...string) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for {
		for _, line := range mockLog.GetOutput(ldlog.Info) {
			matches := true
			for _, substring := range substrings {
				if !strings.Contains(line, substring) {
					matches = false
					break
				}
			}
			if matches {
				return
			}
		}
		if time.Now().After(deadline) {
			require.FailNow(t, "timed out waiting for an Info line containing "+strings.Join(substrings, " and "),
				"Info output: %v", mockLog.GetOutput(ldlog.Info))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestFileSourceRequiresPaths(t *testing.T) {
	_, err := FileSource().Build(sharedtest.BasicClientContext())
	assert.Error(t, err)
}

func TestFileSourceLoadsInitialDataSynchronously(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{"flagValues": {"flag1": true}, "flags": {"flag2": {"key": "flag2", "version": 3, "on": false}}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path) })

	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 2)
	// The flag-value entry was expanded into a full flag definition.
	require.Len(t, flags["flag1"].Variations, 1)
	assert.Equal(t, 3, flags["flag2"].Version)
}

func TestFileSourceLoadsYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	writeFile(t, path, "flagValues:\n  flag1: true\n")

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path) })

	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 1)
}

func TestFileSourceMergesFilesInConfiguredOrder(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "first.json")
	path2 := filepath.Join(dir, "second.json")
	writeFile(t, path1, `{"flags": {"flag1": {"key": "flag1", "version": 1}}}`)
	writeFile(t, path2, `{"flags": {"flag1": {"key": "flag1", "version": 2}}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) {
		b.FilePaths(path1, path2).DuplicateKeysHandling(DuplicateKeysKeepFirst)
	})

	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 1)
	assert.Equal(t, 1, flags["flag1"].Version)
}

func TestFileSourceDuplicateKeysFailByDefault(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "first.json")
	path2 := filepath.Join(dir, "second.json")
	writeFile(t, path1, `{"flags": {"flag1": {"key": "flag1", "version": 1}}}`)
	writeFile(t, path2, `{"flags": {"flag1": {"key": "flag1", "version": 2}}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path1, path2) })

	sink.requireNoSnapshot(t, 200*time.Millisecond)
}

func TestFileSourceStartsWithMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-yet.json")

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path) })

	// A missing file contributes no overrides. The initial snapshot is empty.
	require.Len(t, flagsByKey(t, sink.requireSnapshot(t)), 0)

	// Once the file appears, the change signal picks it up unprompted.
	writeFile(t, path, `{"flagValues": {"flag1": true}}`)
	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 1)
}

func TestFileSourceMissingFileContributesNoEntries(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.json"), filepath.Join(dir, "second.json")
	writeFile(t, first, `{"flagValues": {"from-first": true}}`)

	// Step 1: one configured file exists and one does not. The existing file applies.
	_, sink := buildFileSource(t, func(b *FileSourceBuilder) {
		b.FilePaths(first, second).PollInterval(MinimumPollInterval)
	})
	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 1)
	require.Contains(t, flags, "from-first")

	// Step 2: the second file appears. Both apply.
	writeFile(t, second, `{"flagValues": {"from-second": true}}`)
	flags = flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 2)

	// Step 3: the second file is deleted. Its overrides are removed.
	require.NoError(t, os.Remove(second))
	flags = flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 1)
	require.Contains(t, flags, "from-first")

	// Step 4: the last file is deleted. The layer is cleared.
	require.NoError(t, os.Remove(first))
	require.Len(t, flagsByKey(t, sink.requireSnapshot(t)), 0)
}

func TestFileSourceLogsOverridesInEffectOnEachChange(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.json"), filepath.Join(dir, "second.json")
	writeFile(t, first, `{"flagValues": {"flag1": true, "flag2": false}, "segments": {"seg": {"key": "seg"}}}`)

	// Step 1: at startup, one file supplies entries and the other is absent.
	_, sink, mockLog := buildFileSourceWithLog(t, func(b *FileSourceBuilder) {
		b.FilePaths(first, second).PollInterval(MinimumPollInterval)
	})
	sink.requireSnapshot(t)
	requireInfoLine(t, mockLog, "Flag overrides in effect: 2 flags, 1 segment", first+": 2 flags, 1 segment", second+": absent")

	// Step 2: the absent file appears with one entry.
	writeFile(t, second, `{"flagValues": {"flag3": true}}`)
	requireInfoLine(t, mockLog, "Flag overrides in effect: 3 flags, 1 segment", second+": 1 flag")

	// Step 3: both files are deleted. Nothing is in effect.
	require.NoError(t, os.Remove(first))
	require.NoError(t, os.Remove(second))
	requireInfoLine(t, mockLog, "Flag overrides: none in effect", first+": absent", second+": absent")
}

func TestFileSourceLogsNoneInEffectAtStartupWithoutFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	_, sink, mockLog := buildFileSourceWithLog(t, func(b *FileSourceBuilder) { b.FilePaths(path) })
	sink.requireSnapshot(t)
	requireInfoLine(t, mockLog, "Flag overrides: none in effect", path+": absent")
}

func TestFileSourceWatchingModeIsQuietWhenFileIsAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	_, sink, mockLog := buildFileSourceWithLog(t, func(b *FileSourceBuilder) {
		b.FilePaths(path).ChangeDetection(Watching)
	})
	sink.requireSnapshot(t)

	time.Sleep(2500 * time.Millisecond)
	assert.Empty(t, mockLog.GetOutput(ldlog.Error))
	assert.Empty(t, mockLog.GetOutput(ldlog.Warn))

	writeFile(t, path, `{"flagValues": {"flag1": true}}`)
	require.Len(t, flagsByKey(t, sink.requireSnapshot(t)), 1)
}

func TestFileSourceWatchingModeReloadsOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{"flagValues": {"flag1": true}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path).ChangeDetection(Watching) })
	sink.requireSnapshot(t)

	writeFile(t, path, `{"flagValues": {"flag1": true, "flag2": false}}`)
	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 2)

	// Removing entries removes them from the snapshot (a reload is a full replacement).
	writeFile(t, path, `{}`)
	flags = flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 0)
}

func TestFileSourcePollingModeReloadsOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{"flagValues": {"flag1": true}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) {
		b.FilePaths(path).ChangeDetection(Polling).PollInterval(MinimumPollInterval)
	})
	sink.requireSnapshot(t)

	// Ensure the rewrite is observable through (modTime, size) even on filesystems with
	// coarse timestamp granularity.
	writeFile(t, path, `{"flagValues": {"flag1": true, "flag2": false}}`)
	newTime := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(path, newTime, newTime))

	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 2)
}

func TestFileSourceRetainsLastGoodDataAcrossMalformedEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{"flagValues": {"flag1": true}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path).ChangeDetection(Watching) })
	sink.requireSnapshot(t)

	// A malformed edit produces no snapshot: the previously applied overrides stay in
	// effect because the sink is never called.
	writeFile(t, path, `{"flagValues"`)
	sink.requireNoSnapshot(t, 300*time.Millisecond)

	// Fixing the file recovers, via the change notification or the failure retry.
	writeFile(t, path, `{"flagValues": {"flag1": false}}`)
	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 1)
}

func TestFileSourcePollsByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{}`)

	source, err := FileSource().FilePaths(path).Build(sharedtest.BasicClientContext())
	require.NoError(t, err)
	impl, ok := source.(*fileOverrideSource)
	require.True(t, ok)
	assert.Equal(t, Polling, impl.changeDetection)
	assert.Equal(t, DefaultPollInterval, impl.pollInterval)
}

func TestFileSourceRejectsUnknownChangeDetection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{}`)

	_, err := FileSource().FilePaths(path).ChangeDetection("notify").Build(sharedtest.BasicClientContext())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notify")
}

func TestFileSourcePollIntervalIsClamped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{}`)

	builder := FileSource().FilePaths(path).PollInterval(time.Millisecond)
	source, err := builder.Build(sharedtest.BasicClientContext())
	require.NoError(t, err)
	impl, ok := source.(*fileOverrideSource)
	require.True(t, ok)
	assert.Equal(t, MinimumPollInterval, impl.pollInterval)
}

func TestFileSourceCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{}`)

	source, _ := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path) })
	require.NoError(t, source.Close())
	require.NoError(t, source.Close())
}

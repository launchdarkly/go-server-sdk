package ldoverrides

import (
	"os"
	"path/filepath"
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
		b.FilePaths(path1, path2).DuplicateKeysHandling(DuplicateKeysIgnoreAllButFirst)
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

	// No snapshot at startup; the client runs with no overrides.
	sink.requireNoSnapshot(t, 200*time.Millisecond)

	// Once the file appears, the watch (or the failure retry) picks it up unprompted.
	writeFile(t, path, `{"flagValues": {"flag1": true}}`)
	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 1)
}

func TestFileSourceWatchModeReloadsOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{"flagValues": {"flag1": true}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path) })
	sink.requireSnapshot(t)

	writeFile(t, path, `{"flagValues": {"flag1": true, "flag2": false}}`)
	flags := flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 2)

	// Removing entries removes them from the snapshot (a reload is a full replacement).
	writeFile(t, path, `{}`)
	flags = flagsByKey(t, sink.requireSnapshot(t))
	require.Len(t, flags, 0)
}

func TestFileSourcePollModeReloadsOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{"flagValues": {"flag1": true}}`)

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) {
		b.FilePaths(path).Watch(false).Poll(true).PollInterval(MinimumPollInterval)
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

	_, sink := buildFileSource(t, func(b *FileSourceBuilder) { b.FilePaths(path) })
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

func TestFileSourcePollIntervalIsClamped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{}`)

	builder := FileSource().FilePaths(path).Poll(true).PollInterval(time.Millisecond)
	source, err := builder.Build(sharedtest.BasicClientContext())
	require.NoError(t, err)
	impl, ok := source.(*fileOverrideSource)
	require.True(t, ok)
	assert.Equal(t, MinimumPollInterval, impl.pollInterval)
}

func TestFileSourceCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	writeFile(t, path, `{}`)

	source, _ := buildFileSource(t, func(b *FileSourceBuilder) {
		b.FilePaths(path).Poll(true)
	})
	require.NoError(t, source.Close())
	require.NoError(t, source.Close())
}

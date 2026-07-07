package filedata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const pollTestInterval = 5 * time.Millisecond

func newPollerFixture(t *testing.T, paths []string) chan struct{} {
	t.Helper()
	changed := make(chan struct{}, 100)
	p := NewPoller(paths, pollTestInterval, func() { changed <- struct{}{} })
	t.Cleanup(p.Close)
	return changed
}

func requireChange(t *testing.T, changed chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(testTimeout):
		require.FailNow(t, "timed out waiting for change detection")
	}
}

func requireNoChange(t *testing.T, changed chan struct{}, duration time.Duration) {
	t.Helper()
	select {
	case <-changed:
		require.FailNow(t, "unexpected change detection")
	case <-time.After(duration):
	}
}

// writeFileWithNewModTime rewrites a file and guarantees the observed (modTime, size) state
// differs from the previous state, so the poller must detect it regardless of filesystem
// timestamp granularity.
func writeFileWithNewModTime(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	newTime := time.Now().Add(time.Duration(len(content)) * time.Second)
	require.NoError(t, os.Chtimes(path, newTime, newTime))
}

func TestPollerDetectsModification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	writeFileWithNewModTime(t, path, "one")
	changed := newPollerFixture(t, []string{path})

	requireNoChange(t, changed, 20*pollTestInterval)

	writeFileWithNewModTime(t, path, "two!")
	requireChange(t, changed)
}

func TestPollerDetectsFileAppearing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	changed := newPollerFixture(t, []string{path})

	requireNoChange(t, changed, 20*pollTestInterval)

	writeFileWithNewModTime(t, path, "created")
	requireChange(t, changed)
}

func TestPollerDetectsFileDisappearing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	writeFileWithNewModTime(t, path, "content")
	changed := newPollerFixture(t, []string{path})

	require.NoError(t, os.Remove(path))
	requireChange(t, changed)
}

func TestPollerWatchesAllFiles(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "one.json")
	path2 := filepath.Join(dir, "two.json")
	writeFileWithNewModTime(t, path1, "one")
	writeFileWithNewModTime(t, path2, "two")
	changed := newPollerFixture(t, []string{path1, path2})

	writeFileWithNewModTime(t, path2, "two-changed")
	requireChange(t, changed)
}

func TestPollerStopsOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	writeFileWithNewModTime(t, path, "one")
	changed := make(chan struct{}, 100)
	p := NewPoller([]string{path}, pollTestInterval, func() { changed <- struct{}{} })
	p.Close()
	p.Close() // idempotent

	writeFileWithNewModTime(t, path, "two!")
	requireNoChange(t, changed, 20*pollTestInterval)
}

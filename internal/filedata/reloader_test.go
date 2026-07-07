package filedata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeout = 5 * time.Second

type reloaderFixture struct {
	path     string
	applied  chan MergeResult
	errored  chan error
	reloader *Reloader
}

func newReloaderFixture(t *testing.T, initialContent string, configure func(*ReloaderConfig)) *reloaderFixture {
	t.Helper()
	f := &reloaderFixture{
		path:    filepath.Join(t.TempDir(), "data.json"),
		applied: make(chan MergeResult, 100),
		errored: make(chan error, 100),
	}
	f.write(t, initialContent)
	cfg := ReloaderConfig{
		Paths:                 []string{f.path},
		DuplicateKeysHandling: DuplicateKeysFail,
		Loggers:               ldlog.NewDisabledLoggers(),
		Apply:                 func(result MergeResult) { f.applied <- result },
		OnError:               func(err error) { f.errored <- err },
	}
	if configure != nil {
		configure(&cfg)
	}
	f.reloader = NewReloader(cfg)
	t.Cleanup(f.reloader.Close)
	return f
}

func (f *reloaderFixture) write(t *testing.T, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(f.path, []byte(content), 0600))
}

func (f *reloaderFixture) requireApplied(t *testing.T) MergeResult {
	t.Helper()
	select {
	case result := <-f.applied:
		return result
	case <-time.After(testTimeout):
		require.FailNow(t, "timed out waiting for Apply")
		return MergeResult{}
	}
}

func (f *reloaderFixture) requireErrored(t *testing.T) error {
	t.Helper()
	select {
	case err := <-f.errored:
		return err
	case <-time.After(testTimeout):
		require.FailNow(t, "timed out waiting for OnError")
		return nil
	}
}

func (f *reloaderFixture) requireQuiet(t *testing.T, duration time.Duration) {
	t.Helper()
	select {
	case <-f.applied:
		require.FailNow(t, "unexpected Apply call")
	case <-f.errored:
		require.FailNow(t, "unexpected OnError call")
	case <-time.After(duration):
	}
}

func TestReloaderInitialLoad(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, nil)
	f.reloader.ReloadNow()
	result := f.requireApplied(t)
	require.Len(t, result.Flags, 1)
	assert.Equal(t, "flag1", result.Flags[0].Key)
}

func TestReloaderReportsFailureAndRetainsNothing(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues"`, nil)
	f.reloader.ReloadNow()
	err := f.requireErrored(t)
	var readErr *ReadError
	require.ErrorAs(t, err, &readErr)
	assert.Equal(t, f.path, readErr.Path)
	f.requireQuiet(t, 50*time.Millisecond)
}

func TestReloaderDebounceCoalescesTriggers(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.DebounceDelay = 20 * time.Millisecond
	})
	f.write(t, `{"flagValues": {"flag1": false}}`)
	for i := 0; i < 20; i++ {
		f.reloader.Trigger()
		time.Sleep(time.Millisecond)
	}
	f.requireApplied(t)
	// All of the triggers, arriving within the settle window, coalesced into one reload.
	f.requireQuiet(t, 100*time.Millisecond)
}

func TestReloaderRetriesAfterFailureWithoutFurtherTriggers(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.RetryDelay = 20 * time.Millisecond
	})
	f.reloader.ReloadNow()
	f.requireApplied(t)

	f.write(t, `{"flagValues"`)
	f.reloader.Trigger()
	f.requireErrored(t)

	// Fix the file without triggering; only the automatic retry can observe the fix.
	f.write(t, `{"flagValues": {"flag1": false}}`)
	f.requireApplied(t)
}

func TestReloaderStopsRetryingAfterSuccess(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues"`, func(cfg *ReloaderConfig) {
		cfg.RetryDelay = 10 * time.Millisecond
		cfg.SkipUnchanged = true
	})
	f.reloader.ReloadNow()
	f.requireErrored(t)

	f.write(t, `{"flagValues": {"flag1": true}}`)
	f.requireApplied(t)

	// After the successful reload there are no further attempts (a retry would re-apply or
	// re-error; with SkipUnchanged set, a stray retry of identical content stays silent, so
	// also verify via a changed file that no reload happens without a trigger).
	f.write(t, `{"flagValues": {"flag1": false}}`)
	f.requireQuiet(t, 100*time.Millisecond)
}

func TestReloaderSkipUnchanged(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.SkipUnchanged = true
	})
	f.reloader.ReloadNow()
	f.requireApplied(t)

	f.reloader.Trigger()
	f.requireQuiet(t, 100*time.Millisecond)

	f.write(t, `{"flagValues": {"flag1": false}}`)
	f.reloader.Trigger()
	f.requireApplied(t)
}

func TestReloaderAppliesEveryReloadWhenSkipUnchangedIsOff(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, nil)
	f.reloader.ReloadNow()
	f.requireApplied(t)
	f.reloader.Trigger()
	f.requireApplied(t)
}

func TestReloaderMergesMultipleFilesInOrder(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "first.json")
	path2 := filepath.Join(dir, "second.json")
	require.NoError(t, os.WriteFile(path1, []byte(`{"flagValues": {"flag1": "first"}}`), 0600))
	require.NoError(t, os.WriteFile(path2, []byte(`{"flagValues": {"flag1": "second"}}`), 0600))

	applied := make(chan MergeResult, 10)
	r := NewReloader(ReloaderConfig{
		Paths:                 []string{path1, path2},
		DuplicateKeysHandling: DuplicateKeysIgnoreAllButFirst,
		Loggers:               ldlog.NewDisabledLoggers(),
		Apply:                 func(result MergeResult) { applied <- result },
	})
	t.Cleanup(r.Close)
	r.ReloadNow()

	select {
	case result := <-applied:
		require.Len(t, result.Flags, 1)
	case <-time.After(testTimeout):
		require.FailNow(t, "timed out waiting for Apply")
	}
}

func TestReloaderDoesNothingAfterClose(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, nil)
	f.reloader.ReloadNow()
	f.requireApplied(t)
	f.reloader.Close()
	f.reloader.Close() // idempotent
	f.reloader.Trigger()
	f.requireQuiet(t, 50*time.Millisecond)
}

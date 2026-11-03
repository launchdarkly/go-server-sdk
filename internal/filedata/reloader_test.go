package filedata

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"

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
	return newReloaderFixtureAt(t, "data.json", initialContent, configure)
}

func newReloaderFixtureAt(
	t *testing.T,
	fileName string,
	initialContent string,
	configure func(*ReloaderConfig),
) *reloaderFixture {
	t.Helper()
	f := &reloaderFixture{
		path:    filepath.Join(t.TempDir(), fileName),
		applied: make(chan MergeResult, 100),
		errored: make(chan error, 100),
	}
	f.write(t, initialContent)
	cfg := ReloaderConfig{
		Paths:                 []string{f.path},
		DuplicateKeysHandling: DuplicateKeysFail,
		MakeFlagWithValue:     MakeOffFlagWithValue,
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

func (f *reloaderFixture) requireAppliedWithin(t *testing.T, duration time.Duration) MergeResult {
	t.Helper()
	select {
	case result := <-f.applied:
		return result
	case <-time.After(duration):
		require.FailNow(t, "no Apply within "+duration.String())
		return MergeResult{}
	}
}

// requireAppliedAlready returns the result of an Apply that has already happened, such as one
// made by a ReloadNow that has returned.
func (f *reloaderFixture) requireAppliedAlready(t *testing.T) MergeResult {
	t.Helper()
	select {
	case result := <-f.applied:
		return result
	default:
		require.FailNow(t, "the load did not call Apply")
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

func TestReloaderFailsOnMissingPathByDefault(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.Paths = append(cfg.Paths, missing)
	})
	f.reloader.ReloadNow()
	err := f.requireErrored(t)
	var readErr *ReadError
	require.ErrorAs(t, err, &readErr)
	assert.Equal(t, missing, readErr.Path)
	f.requireQuiet(t, 100*time.Millisecond)
}

func TestReloaderSkipsMissingPathsWhenConfigured(t *testing.T) {
	second := filepath.Join(t.TempDir(), "second.json")
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.Paths = append(cfg.Paths, second)
		cfg.SkipMissingPaths = true
		cfg.SkipUnchanged = true
	})

	// Step 1: one file exists and one does not. The reload succeeds with the existing file.
	f.reloader.ReloadNow()
	result := f.requireApplied(t)
	require.Len(t, result.Flags, 1)
	assert.Equal(t, "flag1", result.Flags[0].Key)
	require.Len(t, result.Files, 2)
	assert.Equal(t, FileSummary{Path: f.path, Present: true, Flags: 1}, result.Files[0])
	assert.Equal(t, FileSummary{Path: second, Present: false}, result.Files[1])

	// Step 2: the missing file appears. Its data is merged in.
	require.NoError(t, os.WriteFile(second, []byte(`{"flagValues": {"flag2": true}}`), 0600))
	f.reloader.ReloadNow()
	result = f.requireApplied(t)
	require.Len(t, result.Flags, 2)

	// Step 3: the file is deleted. Its data is gone and the reload still succeeds.
	require.NoError(t, os.Remove(second))
	f.reloader.ReloadNow()
	result = f.requireApplied(t)
	require.Len(t, result.Flags, 1)
	assert.Equal(t, "flag1", result.Flags[0].Key)
	f.requireQuiet(t, 100*time.Millisecond)
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
	// The settle window is much longer than the whole trigger burst, so that a scheduling
	// stall during the burst cannot let the debounce fire early and split the reloads.
	// SkipUnchanged stays off so that every reload is observable as an Apply: with it on,
	// identical file contents would collapse even a completely broken debounce into a
	// single Apply and the test would prove nothing.
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.DebounceDelay = 400 * time.Millisecond
	})
	f.write(t, `{"flagValues": {"flag1": false}}`)
	for i := 0; i < 20; i++ {
		f.reloader.Trigger()
		time.Sleep(time.Millisecond)
	}
	f.requireApplied(t)
	// The burst must coalesce into one reload, with a tolerance of one more: a debounce
	// tick racing a fresh trigger can legitimately produce a single extra serialized
	// reload, but anything beyond that means triggers are not being coalesced.
	extraApplies := 0
	deadline := time.After(600 * time.Millisecond)
	for done := false; !done; {
		select {
		case <-f.applied:
			extraApplies++
		case <-deadline:
			done = true
		}
	}
	assert.LessOrEqual(t, extraApplies, 1, "trigger burst was not coalesced")
}

func TestReloaderReportsIdenticalFailureOnlyOnce(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	f := newReloaderFixture(t, `{"flagValues"`, func(cfg *ReloaderConfig) {
		cfg.RetryDelay = 10 * time.Millisecond
		cfg.Loggers = logCapture.Loggers
	})
	f.reloader.ReloadNow()
	f.requireErrored(t)

	// The automatic retries keep failing identically; the error is neither re-reported to
	// OnError nor re-logged at error level.
	f.requireQuiet(t, 200*time.Millisecond)
	assert.Len(t, logCapture.GetOutput(ldlog.Error), 1)

	// A different failure is a new report.
	f.write(t, `{"flagValues": {bad}}`)
	err := f.requireErrored(t)
	assert.Contains(t, err.Error(), "error parsing file")

	// Success re-arms reporting: the same failure recurring afterward is reported again.
	f.write(t, `{"flagValues": {"flag1": true}}`)
	f.requireApplied(t)
	f.write(t, `{"flagValues"`)
	f.reloader.Trigger()
	f.requireErrored(t)
}

func TestReloaderUnusedSpawnsNoGoroutine(t *testing.T) {
	// A Reloader can be constructed by a component whose lifecycle never uses or closes it,
	// such as a file data source built as a one-shot initializer, so construction alone
	// must not start the worker goroutine.
	before := runtime.NumGoroutine()
	path := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	for range 50 {
		NewReloader(ReloaderConfig{
			Paths:                 []string{path},
			DuplicateKeysHandling: DuplicateKeysFail,
			MakeFlagWithValue:     MakeOffFlagWithValue,
			Loggers:               ldlog.NewDisabledLoggers(),
			Apply:                 func(MergeResult) {},
		})
	}
	// The count is process-global, so goroutines from earlier tests may still be winding
	// down; poll until it settles rather than asserting a single sample.
	deadline := time.Now().Add(testTimeout)
	for runtime.NumGoroutine() > before+2 {
		if time.Now().After(deadline) {
			require.FailNow(t, "constructing unused Reloaders must not accumulate goroutines")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReloaderCloseDoesNotWaitForInFlightReload(t *testing.T) {
	applyEntered := make(chan struct{})
	applyRelease := make(chan struct{})
	path := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"flag1": true}}`), 0600))
	goroutinesBefore := runtime.NumGoroutine()

	r := NewReloader(ReloaderConfig{
		Paths:                 []string{path},
		DuplicateKeysHandling: DuplicateKeysFail,
		MakeFlagWithValue:     MakeOffFlagWithValue,
		Loggers:               ldlog.NewDisabledLoggers(),
		Apply: func(MergeResult) {
			close(applyEntered)
			<-applyRelease
		},
	})

	r.Trigger()
	select {
	case <-applyEntered:
	case <-time.After(testTimeout):
		require.FailNow(t, "timed out waiting for the reload to start")
	}

	// The reload is parked inside Apply; Close must return anyway, because a reload wedged
	// in blocking I/O must not be able to wedge shutdown.
	closed := make(chan struct{})
	go func() {
		r.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(testTimeout):
		require.FailNow(t, "Close blocked on an in-flight reload")
	}

	// Once the parked reload is released, the worker goroutine terminates rather than
	// lingering past Close. Polled inline: spawning goroutines to poll would disturb the
	// count being measured.
	close(applyRelease)
	deadline := time.Now().Add(testTimeout)
	for runtime.NumGoroutine() > goroutinesBefore+1 {
		if time.Now().After(deadline) {
			require.FailNow(t, "worker goroutine did not exit after Close")
		}
		time.Sleep(10 * time.Millisecond)
	}
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

func TestReloaderRecoveryAppliesEvenWhenContentUnchanged(t *testing.T) {
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.SkipUnchanged = true
	})
	f.reloader.ReloadNow()
	f.requireApplied(t)

	// A reload fails; consumers hear OnError and may move to an interrupted state.
	require.NoError(t, os.Remove(f.path))
	f.reloader.Trigger()
	f.requireErrored(t)

	// The files come back with byte-identical content. The success must be applied despite
	// SkipUnchanged, because only Apply tells the consumer the interruption is over.
	f.write(t, `{"flagValues": {"flag1": true}}`)
	f.reloader.Trigger()
	f.requireApplied(t)

	// Once recovered, identical content skips again.
	f.reloader.Trigger()
	f.requireQuiet(t, 100*time.Millisecond)
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
		MakeFlagWithValue:     MakeOffFlagWithValue,
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

func TestReloaderDebounceWindowIsExtendedByEachTrigger(t *testing.T) {
	// The debounce is a settle window: each trigger moves the deadline out again. A
	// stream of notifications spaced closer together than the window must produce no
	// reload while the stream continues, and exactly one reload after it stops. The
	// burst in TestReloaderDebounceCoalescesTriggers fits inside one window, so it
	// cannot tell a settle window from a fixed window.
	const window = 250 * time.Millisecond
	f := newReloaderFixture(t, `{"flagValues": {"flag1": true}}`, func(cfg *ReloaderConfig) {
		cfg.DebounceDelay = window
	})

	stop := time.Now().Add(5 * window)
	for time.Now().Before(stop) {
		f.reloader.Trigger()
		time.Sleep(window / 5)
	}
	select {
	case <-f.applied:
		require.Fail(t, "a reload ran while change notifications were still arriving")
	default:
	}
	f.requireApplied(t)
	f.requireQuiet(t, 2*window)
}

// The retry delay for the tests of a file that is found empty that run on the real clock. The
// retry comes long enough after the change signal for a test to finish rewriting the file
// before it, even on a slow machine.
const emptyFileRetryDelay = 1500 * time.Millisecond

// Shorter than emptyFileRetryDelay, so a load that happens within it after a change signal
// is not the retry.
const beforeTheRetry = time.Second

// The retry delay for the tests that move a fake clock. The retry never runs during such a
// test, so only the test's change signals start loads, and the clock decides how long a file
// has been empty.
const fakeClockRetryDelay = time.Hour

const readAgainLog = "reading it again"

const (
	yamlFlag1 = "flagValues:\n  flag1: true\n"
	yamlFlag2 = "flagValues:\n  flag2: true\n"
	yamlFlag3 = "flagValues:\n  flag3: true\n"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newEmptyFileFixture configures a reloader for a YAML file that defines flag1, with the
// retry delay of the tests of a file that is found empty, and a log that keeps debug output.
func newEmptyFileFixture(t *testing.T, configure func(*ReloaderConfig)) (*reloaderFixture, *ldlogtest.MockLog) {
	t.Helper()
	return newEmptyFileFixtureAt(t, "data.yaml", yamlFlag1, configure)
}

func newEmptyFileFixtureAt(
	t *testing.T,
	fileName string,
	initialContent string,
	configure func(*ReloaderConfig),
) (*reloaderFixture, *ldlogtest.MockLog) {
	t.Helper()
	mockLog := ldlogtest.NewMockLog()
	mockLog.Loggers.SetMinLevel(ldlog.Debug)
	f := newReloaderFixtureAt(t, fileName, initialContent, func(cfg *ReloaderConfig) {
		cfg.Loggers = mockLog.Loggers
		cfg.RetryDelay = emptyFileRetryDelay
		if configure != nil {
			configure(cfg)
		}
	})
	return f, mockLog
}

// newFakeClockFixture is like newEmptyFileFixture, but the reloader reads a fake clock and its
// retry never runs during the test.
func newFakeClockFixture(
	t *testing.T,
	configure func(*ReloaderConfig),
) (*reloaderFixture, *ldlogtest.MockLog, *fakeClock) {
	t.Helper()
	return newFakeClockFixtureAt(t, "data.yaml", yamlFlag1, configure)
}

func newFakeClockFixtureAt(
	t *testing.T,
	fileName string,
	initialContent string,
	configure func(*ReloaderConfig),
) (*reloaderFixture, *ldlogtest.MockLog, *fakeClock) {
	t.Helper()
	f, mockLog := newEmptyFileFixtureAt(t, fileName, initialContent, func(cfg *ReloaderConfig) {
		cfg.RetryDelay = fakeClockRetryDelay
		if configure != nil {
			configure(cfg)
		}
	})
	clock := &fakeClock{now: time.Unix(1_000_000, 0)}
	f.reloader.now = clock.Now
	return f, mockLog, clock
}

func flagKeys(result MergeResult) []string {
	keys := make([]string, 0, len(result.Flags))
	for _, flag := range result.Flags {
		keys = append(keys, flag.Key)
	}
	slices.Sort(keys)
	return keys
}

func requireLogged(t *testing.T, mockLog *ldlogtest.MockLog, level ldlog.LogLevel, pattern string) {
	t.Helper()
	require.Eventually(t, func() bool { return mockLog.HasMessageMatch(level, pattern) }, testTimeout,
		10*time.Millisecond, "no %s log matching %q", level, pattern)
}

func holdCount(mockLog *ldlogtest.MockLog) int {
	count := 0
	for _, line := range mockLog.GetOutput(ldlog.Debug) {
		if strings.Contains(line, readAgainLog) {
			count++
		}
	}
	return count
}

// requireHeldBack waits for a load to hold back the given number of files, counted by the
// debug lines, and checks that it neither applied nor reported anything. It returns the
// number of files held back so far.
func (f *reloaderFixture) requireHeldBack(t *testing.T, mockLog *ldlogtest.MockLog, before int, files int) int {
	t.Helper()
	require.Eventually(t, func() bool { return holdCount(mockLog) >= before+files }, testTimeout,
		10*time.Millisecond, "the load did not hold back %d file(s)", files)
	require.Equal(t, before+files, holdCount(mockLog))
	select {
	case <-f.applied:
		require.FailNow(t, "a load that held back a file called Apply")
	case <-f.errored:
		require.FailNow(t, "a load that held back a file called OnError")
	default:
	}
	return before + files
}

// emptyInPlace truncates the file without removing it, as a writer that rewrites it in place
// does first. It also moves the file's modification time later than any that it set before,
// so that the write is visible to the reloader even where the file system records the time
// coarsely, or the clock only advances every few milliseconds, as on Windows.
func emptyInPlace(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, nil, 0600))
	info, err := os.Stat(path)
	require.NoError(t, err)
	later := info.ModTime().Add(time.Duration(modTimeShifts.Add(1)) * time.Second)
	require.NoError(t, os.Chtimes(path, later, later))
}

// modTimeShifts counts the calls of emptyInPlace, so that each moves the time further.
var modTimeShifts atomic.Int64

func TestReloaderHoldsBackAnEmptiedFileUntilItIsRewritten(t *testing.T) {
	// A writer that rewrites the file in place truncates it and then writes the new content.
	// A load that finds the file empty after its content was applied keeps the data. An empty
	// file is a YAML document with no entries, so applying it would briefly remove every entry.
	for name, emptied := range map[string]string{"empty": "", "white space only": " \n\t\r\n  "} {
		t.Run(name, func(t *testing.T) {
			f, mockLog := newEmptyFileFixture(t, nil)
			f.reloader.ReloadNow()
			assert.Equal(t, []string{"flag1"}, flagKeys(f.requireApplied(t)))

			f.write(t, emptied)
			f.reloader.Trigger()
			f.requireHeldBack(t, mockLog, 0, 1)
			f.requireQuiet(t, 100*time.Millisecond)

			// No change signal: the retry reads the rewritten file.
			f.write(t, yamlFlag2)
			assert.Equal(t, []string{"flag2"}, flagKeys(f.requireApplied(t)))
			f.requireQuiet(t, 100*time.Millisecond)
		})
	}
}

func TestReloaderAppliesAnEmptiedFileThatStaysEmptyAtTheFirstRetry(t *testing.T) {
	f, mockLog := newEmptyFileFixture(t, nil)
	f.reloader.ReloadNow()
	f.requireApplied(t)

	f.write(t, "")
	f.reloader.Trigger()
	f.requireHeldBack(t, mockLog, 0, 1)

	// The retry comes a tenth of the retry delay after the file has stayed empty, without
	// being written, for the retry delay, so it parses it: a document with no entries. A
	// second retry would come one more retry delay later.
	assert.Empty(t, f.requireAppliedWithin(t, emptyFileRetryDelay+500*time.Millisecond).Flags)
	assert.True(t, mockLog.HasMessageMatch(ldlog.Debug, "Reading file data again after a file was found empty"))
	assert.False(t, mockLog.HasMessageMatch(ldlog.Debug, "after earlier failure"))
}

func TestReloaderRetriesAHeldBackFileWhenItsDelayIsOver(t *testing.T) {
	// A change signal while the file is held back does not postpone the retry: it comes when
	// the file has stayed empty for the retry delay, not a retry delay after the last load.
	const retryDelay = 3 * time.Second
	f, mockLog := newEmptyFileFixture(t, func(cfg *ReloaderConfig) { cfg.RetryDelay = retryDelay })
	f.reloader.ReloadNow()
	f.requireApplied(t)

	f.write(t, "")
	f.reloader.Trigger()
	holds := f.requireHeldBack(t, mockLog, 0, 1)
	time.Sleep(retryDelay / 2)
	f.reloader.Trigger()
	f.requireHeldBack(t, mockLog, holds, 1)

	// Due about 0.6 retry delays from now; a retry armed for a whole retry delay after this
	// load would come one retry delay from now.
	assert.Empty(t, f.requireAppliedWithin(t, retryDelay*85/100).Flags)
}

func TestReloaderRetryHoldsBackAnEmptiedFile(t *testing.T) {
	// A retry left over from a failure can run while the file is being rewritten in place. It
	// holds the file back too, rather than applying it empty.
	f, mockLog := newEmptyFileFixture(t, nil)
	f.reloader.ReloadNow()
	f.requireApplied(t)
	f.write(t, "flagValues: [")
	f.reloader.Trigger()
	f.requireErrored(t)

	// No change signal: only the retry reads the file.
	f.write(t, "")
	f.requireHeldBack(t, mockLog, 0, 1)
	assert.True(t, mockLog.HasMessageMatch(ldlog.Debug, "Retrying flag data load after earlier failure"))
	f.write(t, yamlFlag2)
	assert.Equal(t, []string{"flag2"}, flagKeys(f.requireApplied(t)))
	assert.True(t, mockLog.HasMessageMatch(ldlog.Debug, "Reading file data again after a file was found empty"))
}

func TestReloaderHoldingBackAFileReportsNoFailure(t *testing.T) {
	// Holding a file back reports nothing. A consumer that tracks failures through OnError
	// hears of none, and a failure that was reported once is not reported again when the
	// same content comes back after the file was held back.
	f, mockLog := newEmptyFileFixture(t, nil)
	f.reloader.ReloadNow()
	f.requireApplied(t)
	f.write(t, "flagValues: [")
	f.reloader.Trigger()
	f.requireErrored(t)

	// No change signal: the retry finds the file emptied and holds it back.
	f.write(t, "")
	f.requireHeldBack(t, mockLog, 0, 1)
	f.write(t, "flagValues: [")

	// The next retry fails as before the file was emptied, which is a repeat.
	requireLogged(t, mockLog, ldlog.Debug, "Unable to load flags")
	f.requireQuiet(t, 100*time.Millisecond)
	assert.Len(t, mockLog.GetOutput(ldlog.Error), 1)
}

func TestReloaderDoesNotHoldBackAFileThatIsEmptyFromTheStart(t *testing.T) {
	// An empty file next to the data, such as a placeholder for later entries, has never had
	// content in effect. It does not hold back the changes of the other file.
	placeholder := filepath.Join(t.TempDir(), "placeholder.yaml")
	require.NoError(t, os.WriteFile(placeholder, nil, 0600))
	f, mockLog := newEmptyFileFixture(t, func(cfg *ReloaderConfig) {
		cfg.Paths = append(cfg.Paths, placeholder)
	})
	f.reloader.ReloadNow()
	f.requireApplied(t)

	f.write(t, yamlFlag2)
	f.reloader.Trigger()
	assert.Equal(t, []string{"flag2"}, flagKeys(f.requireAppliedWithin(t, beforeTheRetry)))
	assert.Equal(t, 0, holdCount(mockLog))
}

func TestReloaderAppliesAFileThatIsEmptyAtTheInitialLoad(t *testing.T) {
	f, _ := newEmptyFileFixture(t, nil)
	f.write(t, "")

	// The initial load applies what it reads, without waiting for a retry.
	f.reloader.ReloadNow()
	assert.Empty(t, f.requireAppliedAlready(t).Flags)
}

func TestReloaderDoesNotHoldBackAFileThatWasMissing(t *testing.T) {
	f, _ := newEmptyFileFixture(t, func(cfg *ReloaderConfig) { cfg.SkipMissingPaths = true })
	f.reloader.ReloadNow()
	f.requireApplied(t)
	require.NoError(t, os.Remove(f.path))
	f.reloader.Trigger()
	assert.Empty(t, f.requireApplied(t).Flags)

	// A missing file contributed no content, so the empty file that replaces it is applied
	// right away.
	require.NoError(t, os.WriteFile(f.path, nil, 0600))
	f.reloader.Trigger()
	assert.Empty(t, f.requireAppliedWithin(t, beforeTheRetry).Flags)
}

func TestReloaderParsesAnEmptiedFileRightAwayWithoutRetries(t *testing.T) {
	f, _ := newEmptyFileFixture(t, func(cfg *ReloaderConfig) { cfg.RetryDelay = 0 })
	f.reloader.ReloadNow()
	f.requireApplied(t)

	// Without a retry, nothing would read the file again, so the load applies it as it is.
	f.write(t, "")
	f.reloader.ReloadNow()
	assert.Empty(t, f.requireAppliedAlready(t).Flags)
}

// requireLoadHolds runs a load synchronously and checks that it held back the given number
// of files and neither applied nor reported anything. It returns the number of files held
// back so far.
func (f *reloaderFixture) requireLoadHolds(t *testing.T, mockLog *ldlogtest.MockLog, before int, files int) int {
	t.Helper()
	f.reloader.ReloadNow()
	require.Equal(t, before+files, holdCount(mockLog), "the load did not hold back %d file(s)", files)
	f.requireNoCallback(t)
	return before + files
}

// requireNoCallback checks that no Apply or OnError call has happened.
func (f *reloaderFixture) requireNoCallback(t *testing.T) {
	t.Helper()
	select {
	case <-f.applied:
		require.FailNow(t, "unexpected Apply call")
	case <-f.errored:
		require.FailNow(t, "unexpected OnError call")
	default:
	}
}

// requireLoadApplies runs a load synchronously and returns what it applied.
func (f *reloaderFixture) requireLoadApplies(t *testing.T) MergeResult {
	t.Helper()
	f.reloader.ReloadNow()
	return f.requireAppliedAlready(t)
}

func TestReloaderHoldsBackAnEmptiedFileForExactlyTheDelay(t *testing.T) {
	// Any load can find the file empty while it is rewritten: a change signal for another
	// file, a touch, a poll. Every load until the file has stayed empty for the retry delay
	// holds it back.
	f, mockLog, clock := newFakeClockFixture(t, nil)
	f.requireLoadApplies(t)

	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)
	clock.Advance(fakeClockRetryDelay * 95 / 100)
	f.requireLoadHolds(t, mockLog, holds, 1)
	clock.Advance(fakeClockRetryDelay * 5 / 100)
	assert.Empty(t, f.requireLoadApplies(t).Flags)
}

func TestReloaderDoesNotHoldBackAFileWhoseEmptyStateIsInEffect(t *testing.T) {
	f, mockLog, clock := newFakeClockFixture(t, nil)
	f.requireLoadApplies(t)
	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)
	clock.Advance(fakeClockRetryDelay)
	assert.Empty(t, f.requireLoadApplies(t).Flags)

	// The file is written again, and still empty. Its empty state is in effect, so there is
	// nothing to keep.
	emptyInPlace(t, f.path)
	assert.Empty(t, f.requireLoadApplies(t).Flags)
	assert.Equal(t, holds, holdCount(mockLog))
}

func TestReloaderHoldsBackAFileWithNoYAMLDocument(t *testing.T) {
	// A read can see only the start of a YAML file that is being written, such as a comment
	// or the document marker. It has no document, so it has no entries, like an empty file.
	for name, partial := range map[string]string{"comment": "# overrides\n", "document marker": "---\n"} {
		t.Run(name, func(t *testing.T) {
			f, mockLog, _ := newFakeClockFixture(t, nil)
			f.requireLoadApplies(t)
			f.write(t, partial)
			f.requireLoadHolds(t, mockLog, 0, 1)
			f.write(t, yamlFlag2)
			assert.Equal(t, []string{"flag2"}, flagKeys(f.requireLoadApplies(t)))
		})
	}
	t.Run("empty mapping", func(t *testing.T) {
		// {} is a document with no entries, the way to remove a file's entries. It is applied
		// right away.
		f, mockLog, _ := newFakeClockFixture(t, nil)
		f.requireLoadApplies(t)
		f.write(t, "{}")
		assert.Empty(t, f.requireLoadApplies(t).Flags)
		assert.Equal(t, 0, holdCount(mockLog))
	})
}

func TestReloaderRestartsTheDelayWhenAnEmptiedFileIsWrittenAgain(t *testing.T) {
	// Two quick rewrites in place: the second truncation comes before any load reads the
	// content of the first. The file was written since a load last found it empty, so the
	// delay starts over.
	f, mockLog, clock := newFakeClockFixture(t, nil)
	f.requireLoadApplies(t)

	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	f.write(t, yamlFlag2)
	emptyInPlace(t, f.path)
	clock.Advance(fakeClockRetryDelay * 8 / 10)
	holds = f.requireLoadHolds(t, mockLog, holds, 1)

	// Longer than the delay since a load first found the file empty, shorter since it was
	// written.
	clock.Advance(fakeClockRetryDelay * 8 / 10)
	f.requireLoadHolds(t, mockLog, holds, 1)

	f.write(t, yamlFlag3)
	assert.Equal(t, []string{"flag3"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderStopsHoldingBackAFileThatKeepsBeingWrittenAfterFiveDelays(t *testing.T) {
	// A file that is written again before every load, so that every load finds it empty, is
	// not held back for ever.
	f, mockLog, clock := newFakeClockFixture(t, nil)
	f.requireLoadApplies(t)
	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	for range 6 {
		f.write(t, yamlFlag2)
		emptyInPlace(t, f.path)
		clock.Advance(fakeClockRetryDelay * 8 / 10)
		holds = f.requireLoadHolds(t, mockLog, holds, 1)
	}

	// Five retry delays after the first load that held it back, it is applied.
	f.write(t, yamlFlag2)
	emptyInPlace(t, f.path)
	clock.Advance(fakeClockRetryDelay * 8 / 10)
	assert.Empty(t, f.requireLoadApplies(t).Flags)
}

// newTwoFileFakeClockFixture configures a fake clock reloader for two YAML files, the
// fixture's file with flag1 and a second file with flag2, and applies them.
func newTwoFileFakeClockFixture(t *testing.T) (*reloaderFixture, string, *ldlogtest.MockLog, *fakeClock) {
	t.Helper()
	second := filepath.Join(t.TempDir(), "second.yaml")
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	f, mockLog, clock := newFakeClockFixture(t, func(cfg *ReloaderConfig) {
		cfg.Paths = append(cfg.Paths, second)
	})
	require.Equal(t, []string{"flag1", "flag2"}, flagKeys(f.requireLoadApplies(t)))
	return f, second, mockLog, clock
}

func TestReloaderStopsHoldingBackFilesThatAreRewrittenInTurnAfterFiveDelays(t *testing.T) {
	// The two files are rewritten in place in turn, so that every load finds one of them
	// empty. Each of them is read with content in between, which does not restart its bound.
	// The first file reaches its bound first, five retry delays after a load first held it
	// back.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	emptied, written := second, f.path
	for range 7 {
		require.NoError(t, os.WriteFile(written, []byte(yamlFlag3), 0600))
		emptyInPlace(t, emptied)
		clock.Advance(fakeClockRetryDelay * 8 / 10)
		holds = f.requireLoadHolds(t, mockLog, holds, 1)
		emptied, written = written, emptied
	}

	require.NoError(t, os.WriteFile(written, []byte(yamlFlag3), 0600))
	emptyInPlace(t, emptied)
	clock.Advance(fakeClockRetryDelay * 8 / 10)
	assert.Equal(t, []string{"flag3"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderHoldsBackAnEmptiedFileWhenAnotherFileChanges(t *testing.T) {
	// The second file is rewritten in place. A change to the first file starts a load, and
	// later another one, while the second file is still empty. A load applies all of the
	// files or none, so the change to the first file waits too.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	require.NoError(t, os.WriteFile(second, nil, 0600))
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	clock.Advance(fakeClockRetryDelay * 3 / 10)
	f.write(t, yamlFlag3)
	f.requireLoadHolds(t, mockLog, holds, 1)

	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	assert.Equal(t, []string{"flag2", "flag3"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderStartsTheDelayOfEveryEmptiedFileInOneLoad(t *testing.T) {
	// Both files are emptied at once. The load that finds them empty holds back both, so both
	// are applied once the delay is over, rather than one retry delay per file.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	require.NoError(t, os.WriteFile(second, nil, 0600))
	f.requireLoadHolds(t, mockLog, 0, 2)

	clock.Advance(fakeClockRetryDelay)
	assert.Empty(t, f.requireLoadApplies(t).Flags)
}

func TestReloaderStartsANewDelayAndBoundForEachEmptying(t *testing.T) {
	// The file is rewritten in place twice, a while apart, and the second time a poll reads it
	// empty twice. The load that applies the first rewrite ends the file's bound, so the time
	// since the first emptying does not count against the second.
	f, mockLog, clock := newFakeClockFixture(t, nil)
	f.requireLoadApplies(t)

	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)
	f.write(t, yamlFlag2)
	assert.Equal(t, []string{"flag2"}, flagKeys(f.requireLoadApplies(t)))

	clock.Advance(fakeClockRetryDelay * 49 / 10)
	emptyInPlace(t, f.path)
	holds = f.requireLoadHolds(t, mockLog, holds, 1)
	clock.Advance(fakeClockRetryDelay * 3 / 10)
	f.requireLoadHolds(t, mockLog, holds, 1)

	f.write(t, yamlFlag3)
	assert.Equal(t, []string{"flag3"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderRestartsTheDelayWhenTheFileIsReadWithContent(t *testing.T) {
	// The first file is emptied, then written while the second cannot be parsed, so nothing
	// is applied. Then it is emptied again with the same modification time and size as at the
	// first emptying, as a file system with coarse timestamps can record. The read with
	// content still starts its delay over.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	info, err := os.Stat(f.path)
	require.NoError(t, err)
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	clock.Advance(fakeClockRetryDelay / 2)
	f.write(t, yamlFlag3)
	require.NoError(t, os.WriteFile(second, []byte("flagValues: ["), 0600))
	f.reloader.ReloadNow()
	f.requireErrored(t)

	clock.Advance(fakeClockRetryDelay * 6 / 10)
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	f.write(t, "")
	require.NoError(t, os.Chtimes(f.path, info.ModTime(), info.ModTime()))
	f.requireLoadHolds(t, mockLog, holds, 1)
}

func TestReloaderHoldsBackAFileEmptiedAgainAfterContentWhenItsStateLooksTheSame(t *testing.T) {
	// On a file system that records modification times coarsely, a file emptied a second time
	// can look exactly as it did the first time. The read of its content in between still
	// starts its delay over. The second file is kept written while empty, so that no load
	// applies.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	info, err := os.Stat(f.path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(second, nil, 0600))
	holds := f.requireLoadHolds(t, mockLog, 0, 2)

	clock.Advance(fakeClockRetryDelay * 50 / 60)
	f.write(t, yamlFlag1)
	emptyInPlace(t, second)
	holds = f.requireLoadHolds(t, mockLog, holds, 1)

	clock.Advance(fakeClockRetryDelay * 20 / 60)
	f.write(t, "")
	require.NoError(t, os.Chtimes(f.path, info.ModTime(), info.ModTime()))
	emptyInPlace(t, second)
	holds = f.requireLoadHolds(t, mockLog, holds, 2)

	// The second file gets its content back while the first is still within its new delay.
	clock.Advance(fakeClockRetryDelay * 10 / 60)
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	f.requireLoadHolds(t, mockLog, holds, 1)
}

func TestReloaderKeepsParsingAnEmptiedFileAfterItsDelay(t *testing.T) {
	// The first file is emptied while the second cannot be parsed. Holding back the first file
	// does not hide the failure of the second.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	require.NoError(t, os.WriteFile(second, []byte("flagValues: ["), 0600))
	f.reloader.ReloadNow()
	require.ErrorContains(t, f.requireErrored(t), second)

	// After the delay, loads parse the first file, as long as it is not written.
	clock.Advance(fakeClockRetryDelay)
	f.reloader.ReloadNow()
	requireLogged(t, mockLog, ldlog.Debug, "Unable to load flags")
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	assert.Equal(t, []string{"flag2"}, flagKeys(f.requireLoadApplies(t)))
	assert.Equal(t, 0, holdCount(mockLog))
}

func TestReloaderHoldsBackALongEmptyFileAgainWhenItIsWritten(t *testing.T) {
	// The first file has been empty for longer than the delay, and than its bound, while loads
	// failed on the second. When it is written again, still empty, its delay and its bound
	// start over: it stayed empty for the whole delay, so it was no rewrite.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	require.NoError(t, os.WriteFile(second, []byte("flagValues: ["), 0600))
	f.reloader.ReloadNow()
	f.requireErrored(t)
	clock.Advance(fakeClockRetryDelay * 6)
	f.reloader.ReloadNow()

	emptyInPlace(t, f.path)
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	f.requireLoadHolds(t, mockLog, 0, 1)
	f.write(t, yamlFlag3)
	assert.Equal(t, []string{"flag2", "flag3"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderFailedLoadsDoNotRestartTheBoundOfAFileThatIsHeldBack(t *testing.T) {
	// The first file is written again before every load, so that every load finds it empty,
	// and every other load fails on the second file. Each of those loads still holds the first
	// file back, so the failures do not restart its bound.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	for i := range 6 {
		emptyInPlace(t, f.path)
		clock.Advance(fakeClockRetryDelay * 8 / 10)
		if i%2 == 0 {
			// The load fails. Only the first of these identical failures is reported.
			require.NoError(t, os.WriteFile(second, []byte("flagValues: ["), 0600))
			f.reloader.ReloadNow()
			require.Equal(t, holds, holdCount(mockLog))
			select {
			case <-f.applied:
				require.FailNow(t, "a failed load called Apply")
			case <-f.errored:
			default:
			}
			continue
		}
		require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
		holds = f.requireLoadHolds(t, mockLog, holds, 1)
	}

	// 5.6 retry delays after the first file was first held back.
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	emptyInPlace(t, f.path)
	clock.Advance(fakeClockRetryDelay * 8 / 10)
	assert.Equal(t, []string{"flag2"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderKeepsTheBoundOfAFileThatAFailedLoadDidNotRead(t *testing.T) {
	// The second file is written again before every load, so that every load finds it empty,
	// and every other load fails on the first file, before it reads the second. Those loads
	// hold nothing back, but they do not start the bound of a file they did not read over.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	require.NoError(t, os.WriteFile(second, nil, 0600))
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	for i := range 6 {
		emptyInPlace(t, second)
		clock.Advance(fakeClockRetryDelay * 8 / 10)
		if i%2 == 0 {
			f.write(t, "flagValues: [")
			f.reloader.ReloadNow()
			require.Equal(t, holds, holdCount(mockLog))
			select {
			case <-f.applied:
				require.FailNow(t, "a failed load called Apply")
			case <-f.errored:
			default:
			}
			continue
		}
		f.write(t, yamlFlag3)
		holds = f.requireLoadHolds(t, mockLog, holds, 1)
	}

	// 5.6 retry delays after the second file was first held back.
	emptyInPlace(t, second)
	clock.Advance(fakeClockRetryDelay * 8 / 10)
	assert.Equal(t, []string{"flag3"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderHoldsBackAFixOfAFileSavedInPlaceWithAnError(t *testing.T) {
	// An operator saves a file in place with a typo, and some time later saves the fix in
	// place. Each save truncates the file first. The loads that fail on the typo read the file,
	// so the save of the fix gets a delay and a bound of its own.
	cases := []struct {
		name, fileName, good, broken, fixed string
		sameModTime                         bool
	}{
		{"yaml", "data.yaml", yamlFlag1, "flagValues: [", yamlFlag2, false},
		{"json", "data.json", `{"flagValues": {"flag1": true}}`, `{"flagValues": {"flag1": tru`,
			`{"flagValues": {"flag2": true}}`, false},
		{"yaml with the same modification time", "data.yaml", yamlFlag1, "flagValues: [", yamlFlag2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, mockLog, clock := newFakeClockFixtureAt(t, c.fileName, c.good, nil)
			f.requireLoadApplies(t)
			f.write(t, "")
			info, err := os.Stat(f.path)
			require.NoError(t, err)
			holds := f.requireLoadHolds(t, mockLog, 0, 1)

			f.write(t, c.broken)
			f.reloader.ReloadNow()
			f.requireErrored(t)
			for range 6 {
				clock.Advance(fakeClockRetryDelay)
				f.reloader.ReloadNow()
			}

			f.write(t, "")
			if c.sameModTime {
				// As a file system with coarse timestamps can record it.
				require.NoError(t, os.Chtimes(f.path, info.ModTime(), info.ModTime()))
			}
			f.requireLoadHolds(t, mockLog, holds, 1)
			assert.Len(t, mockLog.GetOutput(ldlog.Error), 1)
			f.write(t, c.fixed)
			assert.Equal(t, []string{"flag2"}, flagKeys(f.requireLoadApplies(t)))
		})
	}
}

func TestReloaderStartsTheBoundOfAFileThatFailsToReadOver(t *testing.T) {
	// Without SkipMissingPaths, as for the file data sources, a missing file fails the load.
	// The file is emptied and held back, then deleted for a while, then created empty again.
	// The loads that fail to read it get to it, so its new emptying gets a bound of its own.
	f, mockLog, clock := newFakeClockFixture(t, nil)
	f.requireLoadApplies(t)
	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)

	require.NoError(t, os.Remove(f.path))
	f.reloader.ReloadNow()
	f.requireErrored(t)
	for range 6 {
		clock.Advance(fakeClockRetryDelay)
		f.reloader.ReloadNow()
	}

	emptyInPlace(t, f.path)
	f.requireLoadHolds(t, mockLog, holds, 1)
}

func TestReloaderLoadThatHoldsAFileBackAndThenFailsDoesNotStartBoundsOver(t *testing.T) {
	// The first two files are rewritten in place in turn, so that every load holds one of them
	// back, and every other load then fails on a third file. A load that holds a file back does
	// not start any bound over, even when it fails afterwards, so the first file reaches its
	// bound.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	third := filepath.Join(filepath.Dir(second), "third.yaml")
	require.NoError(t, os.WriteFile(third, []byte(yamlFlag3), 0600))
	f.reloader.cfg.Paths = append(f.reloader.cfg.Paths, third)
	f.requireLoadApplies(t)
	f.write(t, "")
	f.requireLoadHolds(t, mockLog, 0, 1)

	applied := false
	for i := 1; i <= 12 && !applied; i++ {
		clock.Advance(fakeClockRetryDelay / 2)
		if i%2 == 1 {
			f.write(t, yamlFlag1)
			emptyInPlace(t, second)
			require.NoError(t, os.WriteFile(third, []byte("flagValues: ["), 0600))
		} else {
			require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
			emptyInPlace(t, f.path)
			require.NoError(t, os.WriteFile(third, []byte(yamlFlag3), 0600))
		}
		f.reloader.ReloadNow()
		select {
		case <-f.applied:
			applied = true
		default:
		}
	}
	assert.True(t, applied, "the files were held back without end")
}

func TestReloaderFileFoundMissingStartsANewDelayWhenItIsCreatedEmptyAgain(t *testing.T) {
	// The first file is emptied and held back, then deleted, then created empty again, while
	// the second file fails to load, so nothing is applied. Finding the file missing ends its
	// empty state, so its new emptying gets a delay of its own even though it has the same
	// modification time as the first one, as a file system with coarse timestamps can record.
	var second string
	f, mockLog, clock := newFakeClockFixture(t, func(cfg *ReloaderConfig) {
		second = filepath.Join(filepath.Dir(cfg.Paths[0]), "second.yaml")
		cfg.Paths = append(cfg.Paths, second)
		cfg.SkipMissingPaths = true
	})
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	f.requireLoadApplies(t)

	require.NoError(t, os.WriteFile(second, []byte("flagValues: ["), 0600))
	f.write(t, "")
	info, err := os.Stat(f.path)
	require.NoError(t, err)
	f.reloader.ReloadNow()
	f.requireErrored(t)
	require.NoError(t, os.Remove(f.path))
	f.reloader.ReloadNow()

	clock.Advance(fakeClockRetryDelay * 70 / 60)
	require.NoError(t, os.WriteFile(f.path, nil, 0600))
	require.NoError(t, os.Chtimes(f.path, info.ModTime(), info.ModTime()))
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	f.requireLoadHolds(t, mockLog, 0, 1)
}

func TestReloaderAnotherFilesBoundDoesNotCutShortARewrite(t *testing.T) {
	// The first file is written again before every load, so that every load finds it empty,
	// until it reaches its bound. The second file is rewritten in place just before that, and
	// is still held back for its own delay.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	holds := f.requireLoadHolds(t, mockLog, 0, 1)
	for range 5 {
		emptyInPlace(t, f.path)
		clock.Advance(fakeClockRetryDelay * 8 / 10)
		holds = f.requireLoadHolds(t, mockLog, holds, 1)
	}

	// 4.8 retry delays after the first file was first held back, the second file is emptied.
	emptyInPlace(t, f.path)
	emptyInPlace(t, second)
	clock.Advance(fakeClockRetryDelay * 8 / 10)
	holds = f.requireLoadHolds(t, mockLog, holds, 2)

	// The first file's bound is over; the second file has been empty for 0.4 retry delays.
	emptyInPlace(t, f.path)
	clock.Advance(fakeClockRetryDelay * 4 / 10)
	f.requireLoadHolds(t, mockLog, holds, 1)
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	assert.Equal(t, []string{"flag2"}, flagKeys(f.requireLoadApplies(t)))
}

func TestReloaderSlowReadOfAnotherFileDoesNotShortenTheDelay(t *testing.T) {
	// The read of the first file takes most of a retry delay, and the second file is emptied
	// meanwhile. Its delay starts when it is read, not when the load started.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	var slow atomic.Bool
	f.reloader.readFile = func(path string) ([]byte, error) {
		if path == f.path && slow.Load() {
			clock.Advance(fakeClockRetryDelay * 9 / 10)
			require.NoError(t, os.WriteFile(second, nil, 0600))
		}
		return os.ReadFile(path) //nolint:gosec // test file path
	}
	slow.Store(true)
	holds := f.requireLoadHolds(t, mockLog, 0, 1)
	slow.Store(false)

	clock.Advance(fakeClockRetryDelay * 3 / 10)
	f.requireLoadHolds(t, mockLog, holds, 1)
}

func TestReloaderHoldAsksForTheRetryWhenTheFirstHeldFileIsDue(t *testing.T) {
	// A load that holds files back asks for a retry when the first of them has stayed empty
	// for the retry delay, or has been held back for as long as it may be, plus a tenth of the
	// retry delay. A failure asks for the retry delay.
	const delay = fakeClockRetryDelay
	f, _, clock := newFakeClockFixture(t, nil)
	require.Equal(t, time.Duration(0), f.reloader.reload(false))
	f.requireAppliedAlready(t)

	// The first hold asks for the whole delay, a later one for the rest of it.
	f.write(t, "")
	assert.Equal(t, delay+delay/10, f.reloader.reload(false))
	clock.Advance(delay * 6 / 10)
	assert.Equal(t, delay*4/10+delay/10, f.reloader.reload(false))

	// Written again before every load: the delay starts over each time, until the file's bound
	// comes first.
	for range 4 {
		emptyInPlace(t, f.path)
		assert.Equal(t, delay+delay/10, f.reloader.reload(false))
		clock.Advance(delay)
	}
	// 4.6 retry delays after the file was first held back.
	emptyInPlace(t, f.path)
	assert.Equal(t, delay*4/10+delay/10, f.reloader.reload(false))

	f.write(t, "flagValues: [")
	assert.Equal(t, delay, f.reloader.reload(false))
}

func TestReloaderHoldAsksForTheRetryOfTheFileThatIsDueFirst(t *testing.T) {
	// The first file was emptied earlier than the second, so its delay is over first.
	const delay = fakeClockRetryDelay
	f, second, _, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	assert.Equal(t, delay+delay/10, f.reloader.reload(false))
	clock.Advance(delay * 3 / 10)
	require.NoError(t, os.WriteFile(second, nil, 0600))
	assert.Equal(t, delay*7/10+delay/10, f.reloader.reload(false))
}

func TestReloaderHoldAsksForTheRetryFromTheEndOfTheLoad(t *testing.T) {
	const delay = fakeClockRetryDelay
	t.Run("rest of the delay", func(t *testing.T) {
		// The read of the second file takes half a retry delay. The retry is measured from the
		// end of the load.
		f, second, _, clock := newTwoFileFakeClockFixture(t)
		f.reloader.readFile = func(path string) ([]byte, error) {
			if path == second {
				clock.Advance(delay / 2)
			}
			return os.ReadFile(path) //nolint:gosec // test file path
		}
		f.write(t, "")
		assert.Equal(t, delay/2+delay/10, f.reloader.reload(false))
	})
	t.Run("delay over during the load", func(t *testing.T) {
		// The read of the file after the held one takes longer than what is left of the held
		// file's delay, so the retry is due right away, after the margin.
		f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
		f.write(t, "")
		f.requireLoadHolds(t, mockLog, 0, 1)
		clock.Advance(delay * 57 / 60)
		f.reloader.readFile = func(path string) ([]byte, error) {
			if path == second {
				clock.Advance(delay * 6 / 60)
			}
			return os.ReadFile(path) //nolint:gosec // test file path
		}
		assert.Equal(t, delay/10, f.reloader.reload(false))
	})
}

func TestReloaderAppliesFilesEmptiedInStaggeredTurnsAfterTheBound(t *testing.T) {
	// Two files stay empty and are truncated again in turn, so that whenever one of them has
	// stayed empty for the whole delay, the other is still within its delay and holds the load
	// back. A file that settles so does not get a fresh bound while the other is held back.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	f.write(t, "")
	f.requireLoadHolds(t, mockLog, 0, 1)
	applied := false
	for i := 1; i <= 14 && !applied; i++ {
		clock.Advance(fakeClockRetryDelay * 54 / 60)
		if i%2 == 1 {
			emptyInPlace(t, second)
		} else {
			emptyInPlace(t, f.path)
		}
		f.reloader.ReloadNow()
		// The retry that the hold arms, a little after the other file's delay is over.
		clock.Advance(fakeClockRetryDelay * 12 / 60)
		f.reloader.ReloadNow()
		clock.Advance(-fakeClockRetryDelay * 12 / 60)
		select {
		case <-f.applied:
			applied = true
		default:
		}
	}
	assert.True(t, applied, "the files were held back without end")
}

func TestReloaderStartsTheBoundOfAFileHeldBackDuringAFailureOverOnceItIsReadAgain(t *testing.T) {
	// While the second file fails to load, the first is rewritten once and its gap is held
	// back. Later, the second file is fixed while the first is in the middle of another
	// rewrite, which is held back with a bound of its own.
	f, second, mockLog, clock := newTwoFileFakeClockFixture(t)
	require.NoError(t, os.WriteFile(second, []byte("flagValues: ["), 0600))
	f.write(t, "")
	f.reloader.ReloadNow()
	f.requireErrored(t)
	f.write(t, yamlFlag1)
	f.reloader.ReloadNow()
	clock.Advance(fakeClockRetryDelay * 6)
	f.reloader.ReloadNow()

	require.NoError(t, os.WriteFile(second, []byte(yamlFlag2), 0600))
	f.write(t, "")
	f.requireLoadHolds(t, mockLog, 0, 1)
}

func TestReloaderArmsTheRetryThatTheHoldAsksFor(t *testing.T) {
	// The retry is armed for what the hold asks for, whether the load came from a change
	// signal or from ReloadNow.
	const delay = fakeClockRetryDelay
	requireArmed := func(t *testing.T, f *reloaderFixture, expected time.Duration) {
		t.Helper()
		require.Eventually(t, func() bool { return f.reloader.lastArmedRetryDelay.Load() == int64(expected) },
			testTimeout, 10*time.Millisecond, "the retry was not armed for %s", expected)
	}
	t.Run("change signal", func(t *testing.T) {
		f, mockLog, clock := newFakeClockFixture(t, nil)
		f.requireLoadApplies(t)
		f.write(t, "")
		f.reloader.Trigger()
		holds := f.requireHeldBack(t, mockLog, 0, 1)
		requireArmed(t, f, delay+delay/10)
		clock.Advance(delay * 6 / 10)
		f.reloader.Trigger()
		f.requireHeldBack(t, mockLog, holds, 1)
		requireArmed(t, f, delay*4/10+delay/10)
	})
	t.Run("ReloadNow", func(t *testing.T) {
		f, mockLog, clock := newFakeClockFixture(t, nil)
		f.requireLoadApplies(t)
		clock.Advance(delay)
		f.write(t, "")
		clock.Advance(delay / 10)
		f.requireLoadHolds(t, mockLog, 0, 1)
		requireArmed(t, f, delay+delay/10)
	})
}

func TestReloaderHoldsBackAFileWhoseContentWasMovedFromAnotherFile(t *testing.T) {
	// The content of one configured file is moved to the other (mv second.yaml data.yaml). The
	// files' combined content is the same, so the load is skipped as unchanged, but the data in
	// effect now comes from the file it was moved to. A rewrite of that file in place holds it
	// back like any other.
	var second string
	f, mockLog, _ := newFakeClockFixture(t, func(cfg *ReloaderConfig) {
		second = filepath.Join(filepath.Dir(cfg.Paths[0]), "second.yaml")
		cfg.Paths = append(cfg.Paths, second)
		cfg.SkipMissingPaths = true
		cfg.SkipUnchanged = true
	})
	require.NoError(t, os.Remove(f.path))
	require.NoError(t, os.WriteFile(second, []byte(yamlFlag1), 0600))
	require.Equal(t, []string{"flag1"}, flagKeys(f.requireLoadApplies(t)))

	require.NoError(t, os.Rename(second, f.path))
	f.reloader.ReloadNow()
	require.True(t, mockLog.HasMessageMatch(ldlog.Debug, regexp.QuoteMeta(second)+" does not exist"))
	f.requireNoCallback(t)

	// The file is rewritten in place: truncated, then written after a load has found it empty.
	f.write(t, "")
	f.requireLoadHolds(t, mockLog, 0, 1)
	f.write(t, yamlFlag2)
	assert.Equal(t, []string{"flag2"}, flagKeys(f.requireLoadApplies(t)))
}

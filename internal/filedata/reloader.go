package filedata

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
)

const (
	// DefaultDebounceDelay is a settle window long enough to coalesce the burst of change
	// notifications produced by a single file edit, and short enough to stay responsive.
	DefaultDebounceDelay = 100 * time.Millisecond
	// DefaultRetryDelay bounds how long a failed reload can go uncorrected when no further
	// change notification arrives, for example when the failure came from reading a file
	// mid-write. Reading a local file is cheap, so this can be short.
	DefaultRetryDelay = time.Second
)

// ReloaderConfig configures a Reloader.
type ReloaderConfig struct {
	// Paths is the list of files to load, already resolved to absolute paths. The order is
	// significant: it determines which file wins under the duplicate-key handling.
	Paths []string
	// DuplicateKeysHandling determines what happens when the same key appears in more than
	// one file.
	DuplicateKeysHandling DuplicateKeysHandling
	// MakeFlagWithValue expands each flag-key-to-value entry into a full flag definition.
	// Each source supplies the form its consumers expect. It must not be nil.
	MakeFlagWithValue FlagValueExpander
	// SkipMissingPaths, when true, treats a configured file that does not exist as a file
	// with no content. The reload succeeds with the data of the files that exist. When
	// false, a missing file fails the reload like any other read error.
	SkipMissingPaths bool
	// Loggers receives log output about reloads and failures.
	Loggers ldlog.Loggers
	// Apply is invoked with each successfully merged result. Calls are serialized on the
	// reloader's own goroutine (or, for ReloadNow, under the same lock), so implementations
	// do not need their own synchronization against other reloads. Apply and OnError must
	// not call back into Close.
	Apply func(MergeResult)
	// OnError is invoked when a reload fails, once per distinct failure: with automatic
	// retries, repeats of an identical failure do not re-invoke it (a success re-arms it).
	// The error is a *ReadError when a file could not be read or parsed, or a merge error
	// otherwise. The reloader logs failures itself, so implementations only need to update
	// their own state.
	OnError func(err error)
	// DebounceDelay is how long to wait after a Trigger call for further calls to settle
	// before reloading, coalescing bursts of change notifications into one reload. If zero
	// or negative, each Trigger reloads immediately.
	DebounceDelay time.Duration
	// RetryDelay is how long to wait after a failed reload before automatically retrying,
	// so that a failure observed while a file was being rewritten recovers even if no
	// further change notification arrives. A file whose content is in effect and that is
	// found empty is not applied until it has stayed empty, without being written, for nine
	// tenths of the same delay. If zero or negative, there is no automatic retry, and an
	// empty file is parsed as it is.
	RetryDelay time.Duration
	// SkipUnchanged, if true, suppresses the Apply call when the files' raw contents are
	// byte-identical to the last successfully applied contents.
	SkipUnchanged bool
}

// Reloader owns the reload cycle for a set of data files: it serializes reloads, debounces
// change signals, retains the last good result on failure (by not calling Apply), retries
// after failures, holds back a file that was emptied, and skips no-op applications.
type Reloader struct {
	cfg       ReloaderConfig
	triggerCh chan struct{}
	// retryRequestCh asks the run goroutine to arm the retry timer without performing an
	// immediate reload; it is used when a synchronous ReloadNow fails.
	retryRequestCh chan struct{}
	closeCh        chan struct{}
	closeOnce      sync.Once
	startOnce      sync.Once

	// reloadMu serializes the actual load work between ReloadNow and the run goroutine.
	reloadMu     sync.Mutex
	lastGoodHash []byte
	lastErrorMsg string
	// appliedHadContent records, for each configured path, whether the file contributed
	// content to the data in effect, that is, to the last load that was applied or skipped as
	// unchanged. A missing or empty file contributed none. Guarded by reloadMu.
	appliedHadContent map[string]bool
	// emptyFiles holds the paths whose content is in effect and that loads have found empty
	// since. An entry is removed when a load reads the file with content. See holdBack.
	// Guarded by reloadMu.
	emptyFiles map[string]*emptyFile
	// now returns the current time. Tests replace it.
	now func() time.Time
}

// emptyFile records when loads first found a file empty, and since when it has been empty
// without being written.
type emptyFile struct {
	firstSeen time.Time
	since     time.Time
	modified  modTime
}

// modTime is a file's modification time, or unknown when the file could not be examined.
type modTime struct {
	time  time.Time
	known bool
}

func (m modTime) equal(other modTime) bool {
	return m.known == other.known && m.time.Equal(other.time)
}

func modTimeOf(path string) modTime {
	info, err := os.Stat(path)
	if err != nil {
		return modTime{}
	}
	return modTime{time: info.ModTime(), known: true}
}

// maxHoldRetryDelays bounds the hold of an empty file that keeps being written, so that every
// load finds it empty: it is applied at the latest this many retry delays after a load first
// found it empty.
const maxHoldRetryDelays = 5

// NewReloader creates a Reloader. The caller should perform the initial load with
// ReloadNow, route change signals to Trigger, and call Close when finished.
//
// The worker goroutine starts on the first ReloadNow or Trigger call rather than here:
// a Reloader can be constructed by a component whose lifecycle never uses it (a file data
// source built as a one-shot initializer, whose interface has no Close), and construction
// alone must not leak a goroutine.
func NewReloader(cfg ReloaderConfig) *Reloader {
	return &Reloader{
		cfg:               cfg,
		triggerCh:         make(chan struct{}, 1),
		retryRequestCh:    make(chan struct{}, 1),
		closeCh:           make(chan struct{}),
		appliedHadContent: make(map[string]bool, len(cfg.Paths)),
		emptyFiles:        make(map[string]*emptyFile),
		now:               time.Now,
	}
}

// ensureStarted starts the worker goroutine on first use. The worker may not yet be
// executing when this returns; that is fine, because the signal channels are buffered and
// hold any signal until its select loop comes up.
func (r *Reloader) ensureStarted() {
	if r.isClosed() {
		return
	}
	r.startOnce.Do(func() {
		go r.run()
	})
}

// ReloadNow synchronously loads the files and applies the result (or reports the failure).
// It is used for the initial load; a failure here schedules the same automatic retry as a
// failed triggered reload.
func (r *Reloader) ReloadNow() {
	r.ensureStarted()
	if !r.reload() && r.cfg.RetryDelay > 0 {
		select {
		case r.retryRequestCh <- struct{}{}:
		default:
		}
	}
}

// Trigger signals that the files may have changed and a reload should happen after the
// debounce delay. It never blocks; signals arriving while a reload is already pending are
// coalesced.
func (r *Reloader) Trigger() {
	r.ensureStarted()
	select {
	case r.triggerCh <- struct{}{}:
	default:
	}
}

// Close stops the reloader. It does not wait for a reload that is already in progress --
// one wedged in a blocking file read (an unresponsive network mount, for example) must not
// be able to wedge shutdown -- so such a reload may still deliver its result through Apply
// or OnError shortly after Close returns; consumers must tolerate that, as they always
// have for late reloads. A reload that has not yet reached its callbacks when Close is
// called will not invoke them.
func (r *Reloader) Close() {
	r.closeOnce.Do(func() {
		close(r.closeCh)
	})
}

func (r *Reloader) run() {
	var debounceTimer, retryTimer *time.Timer
	var debounceC, retryC <-chan time.Time
	stopTimer := func(timer *time.Timer) {
		if timer != nil {
			timer.Stop()
		}
	}
	defer func() {
		stopTimer(debounceTimer)
		stopTimer(retryTimer)
	}()

	reloadAndMaybeRetry := func(isRetry bool) {
		if isRetry {
			r.cfg.Loggers.Debug("Retrying flag data load after earlier failure")
		} else {
			r.cfg.Loggers.Info("Reloading flag data after detecting a change")
		}
		// A pending retry is superseded by this reload: it either succeeded, or it failed
		// and arms a fresh retry below.
		stopTimer(retryTimer)
		retryTimer, retryC = nil, nil
		if !r.reload() && r.cfg.RetryDelay > 0 {
			retryTimer = time.NewTimer(r.cfg.RetryDelay)
			retryC = retryTimer.C
		}
	}

	for {
		select {
		case <-r.closeCh:
			return
		case <-r.triggerCh:
			if r.cfg.DebounceDelay <= 0 {
				reloadAndMaybeRetry(false)
				continue
			}
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(r.cfg.DebounceDelay)
				debounceC = debounceTimer.C
			} else {
				// Stop-then-Reset without a drain is safe under this module's Go version:
				// an expired timer's tick cannot be delivered after Reset. At worst a tick
				// already consumed by the debounceC case races a new Trigger, producing one
				// extra serialized reload that skip-unchanged absorbs.
				stopTimer(debounceTimer)
				debounceTimer.Reset(r.cfg.DebounceDelay)
			}
		case <-r.retryRequestCh:
			// A synchronous ReloadNow failed; arm the retry timer without reloading again
			// immediately. An already-armed retry keeps its earlier deadline.
			if retryTimer == nil {
				retryTimer = time.NewTimer(r.cfg.RetryDelay)
				retryC = retryTimer.C
			}
		case <-debounceC:
			debounceTimer, debounceC = nil, nil
			reloadAndMaybeRetry(false)
		case <-retryC:
			retryTimer, retryC = nil, nil
			reloadAndMaybeRetry(true)
		}
	}
}

// reload performs one full load of all configured files. It reports whether the load is
// done, which is what decides whether a retry gets armed: it is false when the load failed
// or a file is held back, and a skipped no-op application counts as done. The whole
// set is re-read on every reload: entries are combined across files in order, so a change
// to one file can alter which file wins for a key.
func (r *Reloader) reload() bool {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	// A trigger already queued when Close was called can still reach here; the select in
	// run() does not prioritize closeCh over triggerCh.
	if r.isClosed() {
		return true
	}

	docs := make([]Document, 0, len(r.cfg.Paths))
	files := make([]FileSummary, 0, len(r.cfg.Paths))
	hadContent := make(map[string]bool, len(r.cfg.Paths))
	// The files that holdBack holds back. The other files are still read, so that every file
	// that is found empty starts its delay in this load.
	var heldBack []string
	hasher := sha256.New()
	for _, path := range r.cfg.Paths {
		// One read feeds both the hash and the parse, so the skip-unchanged hash can never
		// disagree with the content that was actually applied.
		rawData, err := os.ReadFile(path) //nolint:gosec // G304: ok to read file into variable
		if err != nil {
			if r.cfg.SkipMissingPaths && errors.Is(err, fs.ErrNotExist) {
				r.cfg.Loggers.Debugf("File %s does not exist; it contributes no data", path)
				hadContent[path] = false
				files = append(files, FileSummary{Path: path})
				continue
			}
			return r.fail(&ReadError{Err: wrapReadError(err), Path: path})
		}
		hasContent := len(bytes.TrimSpace(rawData)) > 0
		if hasContent {
			delete(r.emptyFiles, path)
		} else if r.holdBack(path) {
			heldBack = append(heldBack, path)
			continue
		}
		hadContent[path] = hasContent
		_, _ = hasher.Write(rawData)
		_, _ = hasher.Write([]byte{0})
		doc, err := parseDocument(rawData)
		if err != nil {
			return r.fail(&ReadError{Err: err, Path: path})
		}
		docs = append(docs, doc)
		files = append(files, FileSummary{Path: path, Present: true})
	}
	if len(heldBack) > 0 {
		return r.readAgainLater(heldBack)
	}

	merged, err := Merge(r.cfg.DuplicateKeysHandling, r.cfg.MakeFlagWithValue, docs...)
	if err != nil {
		return r.fail(err)
	}
	// Documents are the present files in order. Copy their counts onto the file summaries.
	next := 0
	for i := range files {
		if files[i].Present {
			files[i].Flags = merged.Documents[next].Flags
			files[i].Segments = merged.Documents[next].Segments
			next++
		}
	}
	merged.Files = files

	// Close may have happened while the files were being read; deliver nothing in that
	// case. This check is deliberately not atomic with the delivery below: Close must never
	// block on a lock shared with callbacks (blocked I/O or consumer backpressure could
	// then wedge shutdown), so a reload that passes this check can rarely deliver just
	// after Close returns -- see the Close doc.
	if r.isClosed() {
		return true
	}

	// A success right after a failure must apply even when the content is unchanged since
	// the last success: the consumer heard about the failure through OnError and may have
	// moved to an interrupted state, and only Apply tells it things are good again.
	recovering := r.lastErrorMsg != ""
	r.lastErrorMsg = ""
	hash := hasher.Sum(nil)
	// The data of this load is in effect from here on, whether it is applied or is the same
	// as the data already applied.
	for path, had := range hadContent {
		r.appliedHadContent[path] = had
	}
	if r.cfg.SkipUnchanged && !recovering && bytes.Equal(hash, r.lastGoodHash) {
		return true
	}
	r.lastGoodHash = hash
	if r.cfg.Apply != nil {
		r.cfg.Apply(merged)
	}
	return true
}

// holdBack reports whether a load that found the file empty holds it back. A file whose
// content is in effect and that is empty now may be in the middle of an in-place rewrite: a
// writer that truncates the file and then writes the new content. Such a file is not applied
// until it has stayed empty, without being written, for one retry delay, however many loads
// find it empty in that time. A change of its modification time means that it was written
// since a load last found it empty, for example rewritten and truncated again between two
// loads, so the delay starts over. Modification times are only compared with each other,
// never with the clock. A file that keeps being written so that every load finds it empty is
// applied at the latest maxHoldRetryDelays retry delays after a load first found it empty. A
// file whose content is not in effect, such as a placeholder that has always been empty, or
// an emptied file that has been applied, is parsed right away, and so is any empty file when
// retries are off, since nothing would read it again.
func (r *Reloader) holdBack(path string) bool {
	if r.cfg.RetryDelay <= 0 || !r.appliedHadContent[path] {
		return false
	}
	now := r.now()
	modified := modTimeOf(path)
	empty, ok := r.emptyFiles[path]
	if !ok {
		empty = &emptyFile{firstSeen: now, since: now, modified: modified}
		r.emptyFiles[path] = empty
	} else if !empty.modified.equal(modified) {
		empty.since = now
		empty.modified = modified
	}
	// The delay counts as over at nine tenths of the retry delay, as in the other SDKs, whose
	// timers can fire slightly early, so the retry that the hold arms one retry delay later
	// always parses a file that stayed empty. Once the delay is over the file is parsed, and
	// so it is by every later load until it is written.
	delay := r.cfg.RetryDelay - r.cfg.RetryDelay/10
	return now.Sub(empty.since) < delay && now.Sub(empty.firstSeen) < maxHoldRetryDelays*r.cfg.RetryDelay
}

// readAgainLater handles the files that holdBack holds back. Whatever their content would
// parse as, the load keeps the current data and arms the retry, without reporting a failure,
// so the files are read again after the retry delay; the change signal for a write starts a
// new load sooner. Once a file has stayed empty for the delay, a load parses it: a file that
// is still empty is a YAML document with no entries.
func (r *Reloader) readAgainLater(paths []string) bool {
	// Close may have happened while the files were being read; arm no retry in that case.
	if r.isClosed() {
		return true
	}
	for _, path := range paths {
		r.cfg.Loggers.Debugf("File %s is empty, possibly while it is being rewritten; reading it again before applying it",
			path)
	}
	return false
}

func (r *Reloader) fail(err error) bool {
	// Close may have happened while the files were being read; deliver nothing in that
	// case, and report success so no retry is armed.
	if r.isClosed() {
		return true
	}
	// With automatic retries, a persistent failure would repeat the same log entry and the
	// same callback on every attempt, so repeats of an identical failure are demoted to
	// debug level and do not re-invoke OnError. Status-listening consumers therefore see
	// one report per distinct failure rather than one per retry.
	if err.Error() == r.lastErrorMsg {
		r.cfg.Loggers.Debugf("Unable to load flags: %s", err)
		return false
	}
	r.lastErrorMsg = err.Error()
	r.cfg.Loggers.Errorf("Unable to load flags: %s", err)
	if r.cfg.OnError != nil {
		r.cfg.OnError(err)
	}
	return false
}

func (r *Reloader) isClosed() bool {
	select {
	case <-r.closeCh:
		return true
	default:
		return false
	}
}

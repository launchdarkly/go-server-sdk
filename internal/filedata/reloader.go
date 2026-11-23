package filedata

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
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
	// CheckDocument, if not nil, checks each parsed document before the merge. An error fails
	// the reload in the same way as a file that cannot be parsed, with a *ReadError that names
	// the file. Each source supplies the checks its consumers need. The error must be the same
	// for the same document, so that retries recognize a repeated failure.
	CheckDocument func(Document) error
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
	// The error is a *ReadError when a file could not be read or parsed or failed
	// CheckDocument, or a merge error otherwise. The reloader logs failures itself, so implementations only need to update
	// their own state.
	OnError func(err error)
	// DebounceDelay is how long to wait after a Trigger call for further calls to settle
	// before reloading, coalescing bursts of change notifications into one reload. If zero
	// or negative, each Trigger reloads immediately.
	DebounceDelay time.Duration
	// RetryDelay is how long to wait after a failed reload before automatically retrying,
	// so that a failure observed while a file was being rewritten recovers even if no
	// further change notification arrives. A file whose content is in effect and that is
	// found with no entries is not applied until it has stayed so, without being written,
	// for the same delay, or for at most maxHoldRetryDelays delays while it keeps being
	// written. If zero or negative, there is no automatic retry, and such a file is parsed as
	// it is.
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
	// retryRequestCh asks the run goroutine to arm the retry timer, for the delay it carries,
	// without performing an immediate reload; it is used when a synchronous ReloadNow fails or
	// holds a file back.
	retryRequestCh chan time.Duration
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
	// emptyFiles holds the paths whose content is in effect and that loads have found with no
	// entries since the data in effect was applied. Applying data clears it. See holdBack.
	// Guarded by reloadMu.
	emptyFiles map[string]*emptyFile
	// heldBack is true from a load that held files back until the next load starts, so that
	// the retry it armed can say why it reads the files again. Guarded by reloadMu.
	heldBack bool
	// now returns the current time, and readFile reads a file. Tests replace them.
	now      func() time.Time
	readFile func(string) ([]byte, error)
	// lastArmedRetryDelay is the delay of the retry that the run goroutine armed last, in
	// nanoseconds, for tests.
	lastArmedRetryDelay atomic.Int64
}

// emptyFile records what loads found of a file whose content is in effect and that they found
// with no entries.
type emptyFile struct {
	// since is when a load found the file with no entries after a read with entries or a
	// write, and state is the file's state then. since is zero after a read with entries.
	since time.Time
	state fileState
	// firstHeld is when a load first held the file back, or zero when the file has not been
	// held back since a load that held nothing back read it.
	firstHeld time.Time
}

// maxHoldRetryDelays bounds how long a file is held back. A file that keeps being written so
// that every load finds it with no entries, or files that are rewritten in turn, would
// otherwise be held back without end.
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
		retryRequestCh:    make(chan time.Duration, 1),
		closeCh:           make(chan struct{}),
		appliedHadContent: make(map[string]bool, len(cfg.Paths)),
		emptyFiles:        make(map[string]*emptyFile),
		now:               time.Now,
		readFile:          os.ReadFile,
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
	if retryAfter := r.reload(false); retryAfter > 0 {
		select {
		case r.retryRequestCh <- retryAfter:
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

	armRetry := func(retryAfter time.Duration) {
		retryTimer = time.NewTimer(retryAfter)
		retryC = retryTimer.C
		r.lastArmedRetryDelay.Store(int64(retryAfter))
	}

	reloadAndMaybeRetry := func(isRetry bool) {
		if !isRetry {
			r.cfg.Loggers.Info("Reloading flag data after detecting a change")
		}
		// A pending retry is superseded by this reload: it either succeeded, or it failed
		// and arms a fresh retry below.
		stopTimer(retryTimer)
		retryTimer, retryC = nil, nil
		if retryAfter := r.reload(isRetry); retryAfter > 0 {
			armRetry(retryAfter)
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
		case retryAfter := <-r.retryRequestCh:
			// A synchronous ReloadNow failed or held a file back; arm the retry timer without
			// reloading again immediately. An already-armed retry keeps its deadline.
			if retryTimer == nil {
				armRetry(retryAfter)
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

// reload performs one full load of all configured files. It returns how long to wait before
// a retry, or zero when the load is done and no retry is needed: a load that failed asks for
// the retry delay, one that held a file back asks for the time until that file can be
// applied, and a skipped no-op application counts as done. The whole set is re-read on every
// reload: entries are combined across files in order, so a change to one file can alter
// which file wins for a key.
func (r *Reloader) reload(isRetry bool) time.Duration {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	// A trigger already queued when Close was called can still reach here; the select in
	// run() does not prioritize closeCh over triggerCh.
	if r.isClosed() {
		return 0
	}

	if isRetry {
		if r.heldBack {
			r.cfg.Loggers.Debug("Reading file data again after a file was found empty")
		} else {
			r.cfg.Loggers.Debug("Retrying flag data load after earlier failure")
		}
	}
	r.heldBack = false

	docs := make([]Document, 0, len(r.cfg.Paths))
	files := make([]FileSummary, 0, len(r.cfg.Paths))
	hadContent := make(map[string]bool, len(r.cfg.Paths))
	// The files that holdBack holds back. The other files are still read, so that every file
	// that is found empty starts its delay in this load.
	var heldBack []string
	// The files that the load did not hold back, including those that it failed to read or
	// parse. A failure does not stop the reads: the load fails with the first failure, but
	// every file is classified, so that no file's bound is left over from before the failure.
	var reached []string
	var failure error
	fail := func(err error) {
		if failure == nil {
			failure = err
		}
	}
	defer func() {
		// A load that holds nothing back, whether it applies, skips or fails, starts the bound
		// over for every file: such a file is no longer held back, so its next rewrite is a
		// new one.
		if len(heldBack) == 0 {
			for _, path := range reached {
				if empty, ok := r.emptyFiles[path]; ok {
					empty.firstHeld = time.Time{}
				}
			}
		}
	}()
	hasher := sha256.New()
	for _, path := range r.cfg.Paths {
		// One read feeds both the hash and the parse, so the skip-unchanged hash can never
		// disagree with the content that was actually applied.
		rawData, err := r.readFile(path)
		if err != nil {
			if r.cfg.SkipMissingPaths && errors.Is(err, fs.ErrNotExist) {
				r.cfg.Loggers.Debugf("File %s does not exist; it contributes no data", path)
				reached = r.noteNotEmpty(path, reached)
				hadContent[path] = false
				files = append(files, FileSummary{Path: path})
				continue
			}
			reached = append(reached, path)
			fail(&ReadError{Err: wrapReadError(err), Path: path})
			continue
		}
		// A file with no content, or with content that is no YAML document, such as only a
		// comment or "---", has no entries. A read can see that much of a file that is being
		// written.
		var doc Document
		parsed := false
		hasContent := len(bytes.TrimSpace(rawData)) > 0
		if hasContent {
			doc, hasContent, err = parseFileContent(rawData)
			if err != nil {
				reached = r.noteNotEmpty(path, reached)
				fail(&ReadError{Err: err, Path: path})
				continue
			}
			parsed = true
		}
		switch {
		case hasContent:
			reached = r.noteNotEmpty(path, reached)
		case r.holdBack(path, r.now()):
			// The clock is read after this file's own read, so a slow read of another file
			// does not make its emptiness look older than it is.
			heldBack = append(heldBack, path)
			continue
		default:
			reached = append(reached, path)
		}
		hadContent[path] = hasContent
		_, _ = hasher.Write(rawData)
		_, _ = hasher.Write([]byte{0})
		if !parsed {
			doc, err = parseDocument(rawData)
			if err != nil {
				fail(&ReadError{Err: err, Path: path})
				continue
			}
		}
		docs = append(docs, doc)
		files = append(files, FileSummary{Path: path, Present: true})
	}
	// A failure comes first, then a hold, then the merge, which runs only once every file has
	// been read.
	if failure != nil {
		return r.fail(failure)
	}
	if len(heldBack) > 0 {
		return r.readAgainLater(heldBack, r.now())
	}

	if err := r.checkDocuments(docs, files); err != nil {
		return r.fail(err)
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
		return 0
	}

	// A success right after a failure must apply even when the content is unchanged since
	// the last success: the consumer heard about the failure through OnError and may have
	// moved to an interrupted state, and only Apply tells it things are good again.
	recovering := r.lastErrorMsg != ""
	r.lastErrorMsg = ""
	hash := hasher.Sum(nil)
	// The data of this load is in effect from here on, whether it is applied or is the same
	// as the data already applied, so the bounds of the files that were held back start over.
	for path, had := range hadContent {
		r.appliedHadContent[path] = had
	}
	clear(r.emptyFiles)
	if r.cfg.SkipUnchanged && !recovering && bytes.Equal(hash, r.lastGoodHash) {
		return 0
	}
	r.lastGoodHash = hash
	if r.cfg.Apply != nil {
		r.cfg.Apply(merged)
	}
	return 0
}

// noteNotEmpty records a file that a load found with content, even content that fails to
// parse, or found missing, among the files that the load got to. If the file was empty
// before, its empty state has ended, so its next emptying starts a new delay even where a
// coarse modification time makes it look the same.
func (r *Reloader) noteNotEmpty(path string, reached []string) []string {
	if empty, ok := r.emptyFiles[path]; ok {
		empty.since = time.Time{}
	}
	return append(reached, path)
}

// holdBack reports whether a load that found the file with no entries holds it back. A file
// whose content is in effect and that has no entries now may be in the middle of an
// in-place rewrite: a writer that truncates the file and then writes the new content. Such
// a file is held back until it has stayed so, without being written, for the retry delay,
// however many loads find it so in that time. A change of its state, its modification time
// and size read through any symbolic link, means that it was written since a load last
// found it so, for example rewritten and truncated again between two loads, so the delay
// starts over; so does a read with content, even content that fails to parse, or finding
// the file missing. A file is held back for at most maxHoldRetryDelays retry delays from
// when a load first held it back, so that a file that keeps being written while every load
// finds it empty, or files that are rewritten or emptied in turn, are not held back without
// end. The bound is the file's own, and it starts over only at a load that holds nothing
// back, whether that load applies, skips or fails, on this file or another one: every load
// reads every file. While any file holds the loads back, a read with entries, a failure, or
// the file staying empty for the whole delay does not restart it. The bounds of different
// files do not overlap, so n files that each keep being written while empty, started a
// little less than the bound apart, hold the loads back for about n bounds, and a failure
// timed to every expiry of a bound could hold them back without end. A file whose content
// is not in effect, such as a placeholder that has always been empty or an emptied file
// whose empty state has been applied, is parsed right away, and so is any file when retries
// are off, since nothing would read it again.
func (r *Reloader) holdBack(path string, now time.Time) bool {
	if r.cfg.RetryDelay <= 0 || !r.appliedHadContent[path] {
		return false
	}
	state := observe(path)
	empty, ok := r.emptyFiles[path]
	if !ok {
		empty = &emptyFile{}
		r.emptyFiles[path] = empty
	}
	if empty.since.IsZero() || !empty.state.equal(state) {
		empty.since = now
		empty.state = state
	}
	if now.Sub(empty.since) >= r.cfg.RetryDelay {
		return false
	}
	if !empty.firstHeld.IsZero() && now.Sub(empty.firstHeld) >= r.maxHold() {
		return false
	}
	if empty.firstHeld.IsZero() {
		empty.firstHeld = now
	}
	return true
}

func (r *Reloader) maxHold() time.Duration {
	return maxHoldRetryDelays * r.cfg.RetryDelay
}

// readAgainLater handles the files that holdBack holds back. Whatever their content would
// parse as, the load keeps the current data and arms the retry, without reporting a failure.
// The retry comes when the first of the files has stayed so for the retry delay, or has been
// held back for as long as it may be, plus a tenth of the retry delay, so that it finds that
// time over; the change signal for a write starts a load sooner. Once a file has stayed so for
// the delay, a load parses it: a file that is still empty, or holds only a YAML comment, is a
// YAML document with no entries.
func (r *Reloader) readAgainLater(paths []string, now time.Time) time.Duration {
	// Close may have happened while the files were being read; arm no retry in that case.
	if r.isClosed() {
		return 0
	}
	var until time.Time
	for i, path := range paths {
		r.cfg.Loggers.Debugf("File %s is empty, possibly while it is being rewritten; reading it again before applying it",
			path)
		empty := r.emptyFiles[path]
		over := empty.since.Add(r.cfg.RetryDelay)
		if boundOver := empty.firstHeld.Add(r.maxHold()); boundOver.Before(over) {
			over = boundOver
		}
		if i == 0 || over.Before(until) {
			until = over
		}
	}
	r.heldBack = true
	return max(until.Sub(now), 0) + r.cfg.RetryDelay/10
}

func (r *Reloader) fail(err error) time.Duration {
	// Close may have happened while the files were being read; deliver nothing in that
	// case, and arm no retry.
	if r.isClosed() {
		return 0
	}
	// With automatic retries, a persistent failure would repeat the same log entry and the
	// same callback on every attempt, so repeats of an identical failure are demoted to
	// debug level and do not re-invoke OnError. Status-listening consumers therefore see
	// one report per distinct failure rather than one per retry.
	if err.Error() == r.lastErrorMsg {
		r.cfg.Loggers.Debugf("Unable to load flags: %s", err)
		return r.failureRetryDelay()
	}
	r.lastErrorMsg = err.Error()
	r.cfg.Loggers.Errorf("Unable to load flags: %s", err)
	if r.cfg.OnError != nil {
		r.cfg.OnError(err)
	}
	return r.failureRetryDelay()
}

// failureRetryDelay is how long to wait after a failed load before retrying, or zero for no
// retry.
func (r *Reloader) failureRetryDelay() time.Duration {
	return max(r.cfg.RetryDelay, 0)
}

func (r *Reloader) isClosed() bool {
	select {
	case <-r.closeCh:
		return true
	default:
		return false
	}
}

// checkDocuments applies CheckDocument to each document. The documents are the present files
// in order.
func (r *Reloader) checkDocuments(docs []Document, files []FileSummary) error {
	if r.cfg.CheckDocument == nil {
		return nil
	}
	next := 0
	for _, file := range files {
		if !file.Present {
			continue
		}
		if err := r.cfg.CheckDocument(docs[next]); err != nil {
			return &ReadError{Err: err, Path: file.Path}
		}
		next++
	}
	return nil
}

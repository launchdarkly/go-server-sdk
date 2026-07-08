package filedata

import (
	"bytes"
	"crypto/sha256"
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
	// further change notification arrives. If zero or negative, there is no automatic
	// retry.
	RetryDelay time.Duration
	// SkipUnchanged, if true, suppresses the Apply call when the files' raw contents are
	// byte-identical to the last successfully applied contents.
	SkipUnchanged bool
}

// Reloader owns the reload cycle for a set of data files: it serializes reloads, debounces
// change signals, retains the last good result on failure (by not calling Apply), retries
// after failures, and skips no-op applications.
type Reloader struct {
	cfg       ReloaderConfig
	triggerCh chan struct{}
	// retryRequestCh asks the run goroutine to arm the retry timer without performing an
	// immediate reload; it is used when a synchronous ReloadNow fails.
	retryRequestCh chan struct{}
	closeCh        chan struct{}
	closeOnce      sync.Once

	// reloadMu serializes the actual load work between ReloadNow and the run goroutine.
	reloadMu     sync.Mutex
	lastGoodHash []byte
	lastErrorMsg string
}

// NewReloader creates a started Reloader. The caller should perform the initial load with
// ReloadNow, route change signals to Trigger, and call Close when finished.
func NewReloader(cfg ReloaderConfig) *Reloader {
	r := &Reloader{
		cfg:            cfg,
		triggerCh:      make(chan struct{}, 1),
		retryRequestCh: make(chan struct{}, 1),
		closeCh:        make(chan struct{}),
	}
	go r.run()
	return r
}

// ReloadNow synchronously loads the files and applies the result (or reports the failure).
// It is used for the initial load; a failure here schedules the same automatic retry as a
// failed triggered reload.
func (r *Reloader) ReloadNow() {
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
	select {
	case r.triggerCh <- struct{}{}:
	default:
	}
}

// Close stops the reloader. It does not wait for a reload that is already in progress —
// one wedged in a blocking file read (an unresponsive network mount, for example) must not
// be able to wedge shutdown — so such a reload may still deliver its result through Apply
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

// reload performs one full load of all configured files, returning true on success (including
// a skipped no-op application). The whole set is re-read on every reload: entries are combined
// across files in order, so a change to one file can alter which file wins for a key.
func (r *Reloader) reload() bool {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	// A trigger already queued when Close was called can still reach here; the select in
	// run() does not prioritize closeCh over triggerCh.
	if r.isClosed() {
		return true
	}

	docs := make([]Document, 0, len(r.cfg.Paths))
	hasher := sha256.New()
	for _, path := range r.cfg.Paths {
		rawData, err := os.ReadFile(path) //nolint:gosec // G304: ok to read file into variable
		if err != nil {
			return r.fail(&ReadError{Err: wrapReadError(err), Path: path})
		}
		_, _ = hasher.Write(rawData)
		_, _ = hasher.Write([]byte{0})
		doc, err := parseDocument(rawData)
		if err != nil {
			return r.fail(&ReadError{Err: err, Path: path})
		}
		docs = append(docs, doc)
	}

	merged, err := Merge(r.cfg.DuplicateKeysHandling, docs...)
	if err != nil {
		return r.fail(err)
	}

	// Close may have happened while the files were being read; deliver nothing in that case.
	if r.isClosed() {
		return true
	}

	// A success right after a failure must apply even when the content is unchanged since
	// the last success: the consumer heard about the failure through OnError and may have
	// moved to an interrupted state, and only Apply tells it things are good again.
	recovering := r.lastErrorMsg != ""
	r.lastErrorMsg = ""
	hash := hasher.Sum(nil)
	if r.cfg.SkipUnchanged && !recovering && bytes.Equal(hash, r.lastGoodHash) {
		return true
	}
	r.lastGoodHash = hash
	if r.cfg.Apply != nil {
		r.cfg.Apply(merged)
	}
	return true
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

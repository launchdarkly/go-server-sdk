package filedata

import (
	"bytes"
	"crypto/sha256"
	"errors"
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
	// do not need their own synchronization against other reloads.
	Apply func(MergeResult)
	// OnError is invoked for each failed reload. The error is a *ReadError when a file could
	// not be read or parsed, or a merge error otherwise. The reloader logs failures itself,
	// so implementations only need to update their own state.
	OnError func(err error)
	// DebounceDelay is how long to wait after a Trigger call for further calls to settle
	// before reloading, coalescing bursts of change notifications into one reload. If zero,
	// each Trigger reloads immediately.
	DebounceDelay time.Duration
	// RetryDelay is how long to wait after a failed reload before automatically retrying,
	// so that a failure observed while a file was being rewritten recovers even if no
	// further change notification arrives. If zero, there is no automatic retry.
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
	closeCh   chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once

	// reloadMu serializes the actual load work between ReloadNow and the run goroutine.
	reloadMu     sync.Mutex
	lastGoodHash []byte
	lastErrorMsg string
}

// NewReloader creates a started Reloader. The caller should perform the initial load with
// ReloadNow, route change signals to Trigger, and call Close when finished.
func NewReloader(cfg ReloaderConfig) *Reloader {
	r := &Reloader{
		cfg:       cfg,
		triggerCh: make(chan struct{}, 1),
		closeCh:   make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	go r.run()
	return r
}

// ReloadNow synchronously loads the files and applies the result (or reports the failure).
// It is used for the initial load; a failure here schedules the same automatic retry as a
// failed triggered reload.
func (r *Reloader) ReloadNow() {
	if !r.reload() && r.cfg.RetryDelay > 0 {
		r.Trigger()
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

// Close stops the reloader. No reload will call Apply or OnError after Close returns.
func (r *Reloader) Close() {
	r.closeOnce.Do(func() {
		close(r.closeCh)
		<-r.doneCh
		// An in-flight reload holds reloadMu, so acquiring it guarantees the reload has
		// finished before Close returns.
		r.reloadMu.Lock()
		defer r.reloadMu.Unlock()
	})
}

func (r *Reloader) run() {
	defer close(r.doneCh)
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
				stopTimer(debounceTimer)
				debounceTimer.Reset(r.cfg.DebounceDelay)
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
	select {
	case <-r.closeCh:
		return true
	default:
	}

	docs := make([]Document, 0, len(r.cfg.Paths))
	hasher := sha256.New()
	for _, path := range r.cfg.Paths {
		rawData, err := os.ReadFile(path) //nolint:gosec // G304: ok to read file into variable
		if err != nil {
			return r.fail(&ReadError{Err: errors.New("unable to read file: " + err.Error()), Path: path})
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

	r.lastErrorMsg = ""
	hash := hasher.Sum(nil)
	if r.cfg.SkipUnchanged && bytes.Equal(hash, r.lastGoodHash) {
		return true
	}
	r.lastGoodHash = hash
	r.cfg.Apply(merged)
	return true
}

func (r *Reloader) fail(err error) bool {
	// With automatic retries, a persistent failure would repeat the same log entry on every
	// attempt, so repeats of an identical failure are demoted to debug level.
	if err.Error() == r.lastErrorMsg {
		r.cfg.Loggers.Debugf("Unable to load flags: %s", err)
	} else {
		r.lastErrorMsg = err.Error()
		r.cfg.Loggers.Errorf("Unable to load flags: %s", err)
	}
	if r.cfg.OnError != nil {
		r.cfg.OnError(err)
	}
	return false
}

package filedata

import (
	"os"
	"sync"
	"time"
)

// fileState is the observed state of one file, or its absence.
type fileState struct {
	exists  bool
	modTime time.Time
	size    int64
}

// Poller detects changes to a set of files by examining them on a fixed interval. Use it
// where file system change notifications are not available or not reliable, alone or
// together with them. A change to the modification time or the size of any file invokes
// the onChange callback. A file that appears or disappears is also a change. A file that
// os.Stat cannot examine counts as absent.
//
// The poller samples the files once per interval and compares only modification time and
// size. A rewrite that keeps both values is not detected.
//
// Detection is generous. onChange can run for a change that does not alter the effective
// data. Feed it into a Reloader, whose debouncing and skip-unchanged handling absorb the
// excess.
type Poller struct {
	paths     []string
	interval  time.Duration
	onChange  func()
	last      []fileState
	closeCh   chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

// NewPoller creates a started Poller. It examines the files once before it returns, so
// only later changes invoke onChange. Call Close to stop it.
func NewPoller(paths []string, interval time.Duration, onChange func()) *Poller {
	p := &Poller{
		paths:    paths,
		interval: interval,
		onChange: onChange,
		last:     observeAll(paths),
		closeCh:  make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
	go p.run()
	return p
}

// Close stops the poller. It does not wait for an examination or a callback that is in
// progress. A file system that does not respond must not block shutdown. As a result,
// onChange can run one more time shortly after Close returns. Consumers tolerate a late
// call, as they do for a late reload.
func (p *Poller) Close() {
	p.closeOnce.Do(func() {
		close(p.closeCh)
	})
}

func (p *Poller) run() {
	defer close(p.doneCh)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.closeCh:
			return
		case <-ticker.C:
			current := observeAll(p.paths)
			changed := false
			for i := range current {
				if current[i] != p.last[i] {
					changed = true
					break
				}
			}
			p.last = current
			if changed {
				p.onChange()
			}
		}
	}
}

func observeAll(paths []string) []fileState {
	states := make([]fileState, len(paths))
	for i, path := range paths {
		if info, err := os.Stat(path); err == nil {
			states[i] = fileState{exists: true, modTime: info.ModTime(), size: info.Size()}
		}
	}
	return states
}

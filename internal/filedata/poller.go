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

// Poller detects changes to a set of files by examining them on a fixed interval, as an
// alternative or supplement to filesystem change notifications for environments where those
// are unavailable or unreliable. A change to any file's modification time or size — including
// the file appearing or disappearing — invokes the onChange callback. A file that cannot be
// examined is treated as absent, so a file becoming temporarily unreadable and recovering is
// also detected.
//
// Detection is deliberately generous: onChange may be invoked for changes that do not alter
// the effective data, and consumers are expected to feed it into a Reloader, whose debouncing
// and skip-unchanged handling absorb the excess.
type Poller struct {
	paths     []string
	interval  time.Duration
	onChange  func()
	last      []fileState
	closeCh   chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

// NewPoller creates a started Poller. The initial observation happens immediately, so only
// changes after creation invoke onChange. Call Close to stop it.
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

// Close stops the poller. The onChange callback will not be invoked after Close returns.
func (p *Poller) Close() {
	p.closeOnce.Do(func() {
		close(p.closeCh)
		<-p.doneCh
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

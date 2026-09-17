package ldfilewatch

import (
	"fmt"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
)

const retryDuration = time.Second

type fileWatcher struct {
	watcher *fsnotify.Watcher
	loggers ldlog.Loggers
	reload  func()
	paths   []string
	// absPaths is written by setupWatches on the run goroutine and read by the pump goroutine.
	absPathsMu sync.RWMutex
	absPaths   map[string]bool
}

// WatchFiles sets up a mechanism for the file data source to reload its source files whenever one of them has
// been modified. Use it as follows:
//
//	config := Config{
//	    DataSource: ldfiledata.DataSource().
//	        FilePaths(filePaths).
//	        Reloader(ldfilewatch.WatchFiles),
//	}
func WatchFiles(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil { // COVERAGE: can't simulate this condition in unit tests
		return fmt.Errorf("unable to create file watcher: %s", err)
	}
	fw := &fileWatcher{
		watcher:  watcher,
		loggers:  loggers,
		reload:   reload,
		paths:    paths,
		absPaths: make(map[string]bool),
	}
	go fw.run(closeCh)
	return nil
}

func (fw *fileWatcher) run(closeCh <-chan struct{}) {
	// The pump goroutine is the only reader of the watcher's channels. The only operation that
	// can block it is a send to changeCh, and that send never waits. So the watcher backend can
	// always deliver. This matters on Windows: the backend services Add and Close requests on the
	// goroutine that delivers events, and its event channel holds a bounded number of events.
	// If the goroutine that consumes events calls Add while that channel is full, both wait
	// forever and the data source freezes. A burst of writes to a watched file can fill it.
	changeCh := make(chan struct{}, 1)
	pumpDone := make(chan struct{})
	go fw.pump(changeCh, pumpDone)

	retryCh := make(chan struct{}, 1)
	scheduleRetry := func() {
		time.AfterFunc(retryDuration, func() {
			select {
			case retryCh <- struct{}{}: // don't need multiple retries so no need to block
			default: // COVERAGE: can't simulate this condition in unit tests
			}
		})
	}
	for {
		if err := fw.setupWatches(); err != nil {
			fw.loggers.Error(err)
			scheduleRetry()
		}

		// We do the reload here rather than after waiting, even though that means there will be a
		// redundant load when we first start up, because otherwise there's a potential race condition where
		// file changes could happen before we had set up our file watcher.
		fw.reload()

		select {
		case <-closeCh:
			err := fw.watcher.Close()
			if err != nil { // COVERAGE: can't simulate this condition in unit tests
				fw.loggers.Errorf("Error closing Watcher: %s", err)
				return
			}
			// Closing the watcher closes its channels, which ends the pump.
			<-pumpDone
			return
		case <-changeCh:
		case <-retryCh:
		}
	}
}

// pump reads every event and error from the watcher and coalesces the ones that matter into a
// single pending change signal. It returns when the watcher closes its channels.
func (fw *fileWatcher) pump(changeCh chan<- struct{}, done chan<- struct{}) {
	defer close(done)
	signal := func() {
		select {
		case changeCh <- struct{}{}:
		default: // a change is already pending; the reload it causes reads the current contents
		}
	}
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if fw.isWatchedPath(event.Name) {
				signal()
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.loggers.Error(err) // COVERAGE: can't simulate this condition in unit tests
			// An error from the backend, such as a full notification buffer, means that changes
			// may have been dropped. Treat it as a change so the files are read again.
			signal()
		}
	}
}

func (fw *fileWatcher) isWatchedPath(name string) bool {
	fw.absPathsMu.RLock()
	defer fw.absPathsMu.RUnlock()
	return fw.absPaths[name]
}

func (fw *fileWatcher) setupWatches() error {
	for _, p := range fw.paths {
		absDirPath := path.Dir(p)
		realDirPath, err := filepath.EvalSymlinks(absDirPath)
		if err != nil {
			return fmt.Errorf(`unable to evaluate symlinks for "%s": %s`, absDirPath, err)
		}

		realPath := path.Join(realDirPath, path.Base(p))
		fw.absPathsMu.Lock()
		fw.absPaths[realPath] = true
		fw.absPathsMu.Unlock()
		if err = fw.watcher.Add(realPath); err != nil { // COVERAGE: can't simulate this condition in unit tests
			return fmt.Errorf(`unable to watch path "%s": %s`, realPath, err)
		}
		if err = fw.watcher.Add(realDirPath); err != nil { // COVERAGE: can't simulate this in unit tests
			return fmt.Errorf(`unable to watch path "%s": %s`, realDirPath, err)
		}
	}
	return nil
}

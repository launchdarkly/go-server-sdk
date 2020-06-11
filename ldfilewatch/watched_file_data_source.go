package ldfilewatch

import (
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

const retryDuration = time.Second

type fileWatcher struct {
	watcher  *fsnotify.Watcher
	loggers  ldlog.Loggers
	reload   func()
	paths    []string
	absPaths map[string]bool
}

// WatchFiles sets up a mechanism for the file data source to reload its source files whenever one of them has
// been modified. Use it as follows:
//
//     config := Config{
//         DataSource: ldfiledata.DataSource().
//             FilePaths(filePaths).
//             Reloader(ldfilewatch.WatchFiles),
//     }
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

		// We do the reload here rather than after waitForEvents, even though that means there will be a
		// redundant load when we first start up, because otherwise there's a potential race condition where
		// file changes could happen before we had set up our file watcher.
		fw.reload()

		quit := fw.waitForEvents(closeCh, retryCh)
		if quit {
			return
		}
	}
}

func (fw *fileWatcher) setupWatches() error {
	for _, p := range fw.paths {
		absDirPath := path.Dir(p)
		realDirPath, err := filepath.EvalSymlinks(absDirPath)
		if err != nil {
			return fmt.Errorf(`unable to evaluate symlinks for "%s": %s`, absDirPath, err)
		}

		realPath := path.Join(realDirPath, path.Base(p))
		fw.absPaths[realPath] = true
		if err = fw.watcher.Add(realPath); err != nil { // COVERAGE: can't simulate this condition in unit tests
			return fmt.Errorf(`unable to watch path "%s": %s`, realPath, err)
		}
		if err = fw.watcher.Add(realDirPath); err != nil { // COVERAGE: can't simulate this in unit tests
			return fmt.Errorf(`unable to watch path "%s": %s`, realDirPath, err)
		}
	}
	return nil
}

func (fw *fileWatcher) waitForEvents(closeCh <-chan struct{}, retryCh <-chan struct{}) bool {
	for {
		fw.loggers.Warn("waitForEvents")
		select {
		case <-closeCh:
			fw.loggers.Error("got close")
			err := fw.watcher.Close()
			if err != nil { // COVERAGE: can't simulate this condition in unit tests
				fw.loggers.Errorf("Error closing Watcher: %s", err)
			}
			return true
		case event := <-fw.watcher.Events:
			if !fw.absPaths[event.Name] { // COVERAGE: can't simulate this condition in unit tests
				break
			}
			fw.consumeExtraEvents()
			return false
		case err := <-fw.watcher.Errors:
			fw.loggers.Error(err) // COVERAGE: can't simulate this condition in unit tests
		case <-retryCh:
			consumeExtraRetries(retryCh)
			return false
		}
	}
}

func (fw *fileWatcher) consumeExtraEvents() {
	for {
		select {
		case <-fw.watcher.Events: // COVERAGE: can't simulate this condition in unit tests
		default:
			return
		}
	}
}

func consumeExtraRetries(retryCh <-chan struct{}) {
	for {
		select {
		case <-retryCh: // COVERAGE: can't simulate this condition in unit tests
		default:
			return
		}
	}
}

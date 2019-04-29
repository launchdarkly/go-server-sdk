// Package ldfilewatch allows the LaunchDarkly client to read feature flag data from a
// file, with automatic reloading. It should be used in conjunction with the ldfiledata package.
// The two packages are separate so as to avoid bringing additional dependencies for users who
// do not need automatic reloading.
package ldfilewatch

import (
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
)

const retryDuration = time.Second

type fileWatcher struct {
	watcher  *fsnotify.Watcher
	logger   ld.Logger
	reload   func()
	paths    []string
	absPaths map[string]bool
}

// WatchFiles sets up a mechanism for the file data source to reload its source files whenever one of them has
// been modified. Use it as follows:
//
//     factory := ldfiledata.NewFileDataSourceFactory(
//         ldfiledata.FilePaths("./test-data/my-flags.json"),
//         ldfiledata.UseReloader(ldfilewatch.WatchFiles))
func WatchFiles(paths []string, logger ld.Logger, reload func(), closeCh <-chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("Unable to create file watcher: %s", err)
	}
	fw := &fileWatcher{
		watcher:  watcher,
		logger:   logger,
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
			default:
			}
		})
	}
	for {
		if err := fw.setupWatches(); err != nil {
			fw.logger.Printf(err.Error())
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
			return fmt.Errorf(`Unable to evaluate symlinks for "%s": %s`, absDirPath, err)
		}

		realPath := path.Join(realDirPath, path.Base(p))
		fw.absPaths[realPath] = true
		if err = fw.watcher.Add(realPath); err != nil {
			return fmt.Errorf(`Unable to watch path "%s": %s`, realPath, err)
		}
		if err = fw.watcher.Add(realDirPath); err != nil {
			return fmt.Errorf(`Unable to watch path "%s": %s`, realDirPath, err)
		}
	}
	return nil
}

func (fw *fileWatcher) waitForEvents(closeCh <-chan struct{}, retryCh <-chan struct{}) bool {
	for {
		select {
		case <-closeCh:
			err := fw.watcher.Close()
			if err != nil {
				fw.logger.Printf("Error closing Watcher: %s", err)
			}
			return true
		case event := <-fw.watcher.Events:
			if !fw.absPaths[event.Name] {
				break
			}
			fw.consumeExtraEvents()
			return false
		case err := <-fw.watcher.Errors:
			fw.logger.Println("ERROR: ", err)
		case <-retryCh:
			consumeExtraRetries(retryCh)
			return false
		}
	}
}

func (fw *fileWatcher) consumeExtraEvents() {
	for {
		select {
		case <-fw.watcher.Events:
		default:
			return
		}
	}
}

func consumeExtraRetries(retryCh <-chan struct{}) {
	for {
		select {
		case <-retryCh:
		default:
			return
		}
	}
}

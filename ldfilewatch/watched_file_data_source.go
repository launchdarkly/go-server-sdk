// Package ldfilewatch allows the LaunchDarkly client to read feature flag data from
// a file, with automatic reloading.
package ldfilewatch

import (
	"fmt"
	"path"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"

	ld "gopkg.in/launchdarkly/go-client.v4"
	"gopkg.in/launchdarkly/go-client.v4/ldfiledata"
)

// WatchFiles sets up a mechanism for FileDataSource to reload its source files whenever one of them has
// been modified. Use it as follows:
//
//     fileSource, err := ldfiledata.NewFileDataSource(featureStore,
//         ldfiledata.FilePaths("./test-data/my-flags.json"),
//         ldfiledata.UseReloader(ldfilewatch.WatchFiles()))
func WatchFiles() ldfiledata.Reloader {
	return &fileWatchingReloader{
		closeCh: make(chan struct{}),
	}
}

type fileWatchingReloader struct {
	closeOnce sync.Once
	closeCh   chan struct{}
}

// Start is used internally by FileDataSource.
func (fw *fileWatchingReloader) Start(paths []string, logger ld.Logger, reload func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("Unable to create file watcher: %s", err)
	}

	realPaths := map[string]bool{}
	for _, p := range paths {
		absDirPath := path.Dir(p)
		realDirPath, err := filepath.EvalSymlinks(absDirPath)
		if err != nil {
			return fmt.Errorf(`Unable to evaluate symlinks for "%s": %s`, absDirPath, err)
		}

		realPath := path.Join(realDirPath, path.Base(p))
		realPaths[realPath] = true
		if err = watcher.Add(realDirPath); err != nil {
			return fmt.Errorf(`Unable to watch directory "%s" for file "%s": %s`, realDirPath, p, err)
		}
	}

	go func() {
		for {
			for {
				select {
				case <-fw.closeCh:
					err := watcher.Close()
					if err != nil {
						logger.Printf("Error closing Watcher: %s", err)
					}
					return
				case event := <-watcher.Events:
					if realPaths[event.Name] {
						// Consume extra events
					ConsumeExtraEvents:
						for {
							select {
							case <-watcher.Events:
							default:
								break ConsumeExtraEvents
							}
						}
						reload()
					}
				case err := <-watcher.Errors:
					logger.Println("ERROR: ", err)
				}
			}
		}
	}()

	return nil
}

// Close is used internally by FileDataSource.
func (fw *fileWatchingReloader) Close() error {
	fw.closeOnce.Do(func() {
		close(fw.closeCh)
	})
	return nil
}

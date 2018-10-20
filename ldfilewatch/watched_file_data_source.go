// Package ldfilewatch allows the LaunchDarkly client to read feature flag data from
// a file, with automatic reloading.
package ldfilewatch

import (
	"fmt"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	ld "gopkg.in/launchdarkly/go-client.v4"
	"gopkg.in/launchdarkly/go-client.v4/ldfiledata"
)

const retryDuration = time.Second

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
func (fw *fileWatchingReloader) Start(paths []string, logger ld.Logger, reload func() error) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("Unable to create file watcher: %s", err)
	}

	retryChannel := make(chan struct{}, 1)
	scheduleRetry := func() {
		time.AfterFunc(retryDuration, func() {
			select {
			case retryChannel <- struct{}{}: // don't need multiple retries so no need to block
			default:
			}
		})
	}
	go func() {
		for {
			realPaths := map[string]bool{}
			for _, p := range paths {
				absDirPath := path.Dir(p)
				realDirPath, err := filepath.EvalSymlinks(absDirPath)
				if err != nil {
					logger.Printf(`Unable to evaluate symlinks for "%s": %s`, absDirPath, err)
					scheduleRetry()
				}

				realPath := path.Join(realDirPath, path.Base(p))
				realPaths[realPath] = true
				_ = watcher.Add(realPath) // ok if this doesn't find the file: we're still watching the directory

				if err = watcher.Add(realDirPath); err != nil {
					logger.Printf(`Unable to watch directory "%s" for file "%s": %s`, realDirPath, p, err)
					scheduleRetry()
				}
			}

		WaitForUpdates:
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
						break WaitForUpdates
					}
				case err := <-watcher.Errors:
					logger.Println("ERROR: ", err)
				case <-retryChannel:
				ConsumeExtraRetries:
					for {
						select {
						case <-retryChannel:
						default:
							break ConsumeExtraRetries
						}
					}
					break WaitForUpdates
				}
			}
			err := reload()
			if err != nil {
				logger.Printf("Unable to reload flags from data source: %s", err)
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

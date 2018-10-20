// Package ldfilewatch allows the LaunchDarkly client to read feature flag data from
// a file, with automatic reloading.
package ldfilewatch

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	ld "gopkg.in/launchdarkly/go-client.v4"
	"gopkg.in/launchdarkly/go-client.v4/ldfiledata"
)

// WatchedFileDataSource allows the LaunchDarkly client to obtain feature flag data from a file
// or files, and automatically reloads the data if one of the files has changed. To use it, create
// an instance with NewWatchedFileDataSource() and store it in the UpdateProcessor property of
// the LaunchDarkly client configuration before creating the client.
type WatchedFileDataSource struct {
	watcher       *fsnotify.Watcher
	logger        ld.Logger
	store         ld.FeatureStore
	options       []ldfiledata.FileDataSourceOption
	isInitialized bool
	paths         []string
	closeOnce     sync.Once
	closeCh       chan struct{}
}

// NewWatchedFileDataSource creates a new instance of WatchedFileDataSource, allowing the LaunchDarkly
// client to read feature flag data from a file or files.  You should store this instance in the UpdateProcessor
// property of your client configuration before creating the client. It will then work exactly like
// ldfiledata.FileDataSource except that it will reload the flag data whenever it detects a
// change to any of the files.
//
//     featureStore := ld.NewInMemoryFeatureStore(nil)
//     fileSource, err := ldfilewatch.NewWatchedFileDataSource(featureStore,
//         ldfiledata.FilePaths("./test-data/my-flags.json"))
//     ldConfig := ld.DefaultConfig
//     ldConfig.FeatureStore = featureStore
//     ldConfig.UpdateProcessor = fileSource
//     ldClient := ld.MakeCustomClient(mySdkKey, ldConfig, 5*time.Second)
//
// It is important to set the FeatureStore property of your client configuration to the same FeatureStore
// object that you passed to NewWatchedFileDataSource; this is how the component provides flag data to the
// client.
//
// Use FileSourcePaths to specify any number of file paths. The files are not actually loaded until the
// client starts up. At that point, if any file does not exist or cannot be parsed, the WatchedFileDataSource
// will log an error and will not load any data (although it will load it later if you correct the error).
//
// See ldfiledata.NewFileDataSource for details on the format of flag data files.
func NewWatchedFileDataSource(featureStore ld.FeatureStore,
	options ...ldfiledata.FileDataSourceOption) (*WatchedFileDataSource, error) {
	// Ideally we would not need to go through the options twice (here and in NewFileDataSource)
	// but currently the separation between these two constructors makes it necessary.
	paths := make([]string, 0)
	var logger ld.Logger
	for _, o := range options {
		paths = append(paths, o.GetFilePaths()...)
		if o.GetLogger() != nil {
			logger = o.GetLogger()
		}
	}

	if logger == nil {
		logger = log.New(os.Stderr, "[LaunchDarkly WatchedFileDataSource] ", log.LstdFlags)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Unable to create file watcher: %s", err)
	}

	for _, p := range paths {
		_ = watcher.Add(p) // ok if this doesn't work; we'll revisit it in Start()
	}

	ws := &WatchedFileDataSource{
		store:   featureStore,
		options: options,
		logger:  logger,
		watcher: watcher,
		paths:   paths,
		closeCh: make(chan struct{}),
	}
	return ws, nil
}

// Initialized is used internally by the LaunchDarkly client.
func (ws *WatchedFileDataSource) Initialized() bool {
	return ws.isInitialized
}

const retryDuration = time.Second

// Start is used internally by the LaunchDarkly client.
func (ws *WatchedFileDataSource) Start(closeWhenReady chan<- struct{}) {
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
			for _, p := range ws.paths {
				absDirPath := path.Dir(p)
				realDirPath, err := filepath.EvalSymlinks(absDirPath)
				if err != nil {
					ws.logger.Printf(`Unable to evaluate symlinks for "%s": %s`, absDirPath, err)
					scheduleRetry()
				}

				realPath := path.Join(realDirPath, path.Base(p))
				realPaths[realPath] = true
				_ = ws.watcher.Add(realPath) // ok if this doesn't find the file: we're still watching the directory

				if err = ws.watcher.Add(realDirPath); err != nil {
					ws.logger.Printf(`Unable to watch directory "%s" for file "%s": %s`, realDirPath, p, err)
					scheduleRetry()
				}
			}

			baseFp, err := ldfiledata.NewFileDataSource(ws.store, ws.options...)
			if err != nil {
				ws.logger.Printf("Unable to create FileDataSource: %s", err)
				close(closeWhenReady)
				return
			}
			baseCloseWhenReady := make(chan struct{})
			baseFp.Start(baseCloseWhenReady)
			<-baseCloseWhenReady
			if !ws.isInitialized {
				if baseFp.Initialized() {
					ws.isInitialized = true
					close(closeWhenReady)
				}
			}

			// wait for updates
		WaitForUpdates:
			for {
				select {
				case <-ws.closeCh:
					err := baseFp.Close()
					if err != nil {
						ws.logger.Printf("Error closing FileDataSource: %s", err)
					}
					return
				case event := <-ws.watcher.Events:
					if realPaths[event.Name] {
						// Consume extra events
					ConsumeExtraEvents:
						for {
							select {
							case <-ws.watcher.Events:
							default:
								break ConsumeExtraEvents
							}
						}
						break WaitForUpdates
					}
				case err := <-ws.watcher.Errors:
					ws.logger.Println("ERROR: ", err)
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
		}
	}()
}

// Close is called automatically when the client is closed.
func (ws *WatchedFileDataSource) Close() error {
	ws.closeOnce.Do(func() {
		close(ws.closeCh)
	})
	return ws.watcher.Close()
}

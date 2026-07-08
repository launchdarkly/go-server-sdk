package ldfiledata

import (
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/filedata"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

type fileDataSource struct {
	dataSourceUpdates     subsystems.DataSourceUpdateSink
	absFilePaths          []string
	duplicateKeysHandling DuplicateKeysHandling
	reloaderFactory       ReloaderFactory
	loggers               ldlog.Loggers
	isInitialized         bool
	readyCh               chan<- struct{}
	readyOnce             sync.Once
	closeOnce             sync.Once
	// closeReloaderCh is created up front rather than when the reloader starts, so that
	// Close never races with Start assigning it.
	closeReloaderCh chan struct{}
	// reloaderStarted means reload calls may now come from the reloader, which is worth a
	// log line; the initial load is not.
	reloaderStarted bool
}

func newFileDataSourceImpl(
	context subsystems.ClientContext,
	dataSourceUpdates subsystems.DataSourceUpdateSink,
	filePaths []string,
	duplicateKeysHandling DuplicateKeysHandling,
	reloaderFactory ReloaderFactory,
) (subsystems.DataSource, error) {
	abs, err := filedata.AbsFilePaths(filePaths)
	if err != nil {
		// COVERAGE: there's no reliable cross-platform way to simulate an invalid path in unit tests
		return nil, err
	}

	fs := &fileDataSource{
		dataSourceUpdates:     dataSourceUpdates,
		absFilePaths:          abs,
		duplicateKeysHandling: duplicateKeysHandling,
		reloaderFactory:       reloaderFactory,
		loggers:               context.GetLogging().Loggers,
		closeReloaderCh:       make(chan struct{}),
	}
	fs.loggers.SetPrefix("FileDataSource:")
	return fs, nil
}

func (fs *fileDataSource) IsInitialized() bool {
	return fs.isInitialized
}

func (fs *fileDataSource) Start(closeWhenReady chan<- struct{}) {
	fs.readyCh = closeWhenReady
	fs.reload()

	// If there is no reloader, then we signal readiness immediately regardless of whether the
	// data load succeeded or failed.
	if fs.reloaderFactory == nil {
		fs.signalStartComplete(fs.isInitialized)
		return
	}

	// If there is a reloader, and if we haven't yet successfully loaded data, then the
	// readiness signal will happen the first time we do get valid data (in reload).
	fs.reloaderStarted = true
	err := fs.reloaderFactory(fs.absFilePaths, fs.loggers, fs.reload, fs.closeReloaderCh)
	if err != nil {
		fs.loggers.Errorf("Unable to start reloader: %s\n", err)
	}
}

// Reload tells the data source to immediately attempt to reread all of the configured source files
// and update the feature flag state. If any file cannot be loaded or parsed, the flag state will not
// be modified.
func (fs *fileDataSource) reload() {
	if fs.reloaderStarted {
		fs.loggers.Info("Reloading flag data after detecting a change")
	}
	docs := make([]filedata.Document, 0)
	for _, path := range fs.absFilePaths {
		doc, err := filedata.ReadFile(path)
		if err == nil {
			docs = append(docs, doc)
		} else {
			fs.loggers.Errorf("Unable to load flags: %s [%s]", err, path)
			fs.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateInterrupted,
				interfaces.DataSourceErrorInfo{
					Kind:    interfaces.DataSourceErrorKindInvalidData,
					Message: err.Error(),
					Time:    time.Now(),
				})
			return
		}
	}
	merged, err := filedata.Merge(filedata.DuplicateKeysHandling(fs.duplicateKeysHandling), docs...)
	if err == nil {
		storeData := []ldstoretypes.Collection{
			{Kind: datakinds.Features, Items: merged.Flags},
			{Kind: datakinds.Segments, Items: merged.Segments},
		}
		if fs.dataSourceUpdates.Init(storeData) {
			fs.signalStartComplete(true)
			fs.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
		}
	} else {
		fs.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateInterrupted,
			interfaces.DataSourceErrorInfo{
				Kind:    interfaces.DataSourceErrorKindInvalidData,
				Message: err.Error(),
				Time:    time.Now(),
			})
	}
	if err != nil {
		fs.loggers.Error(err)
	}
}

func (fs *fileDataSource) signalStartComplete(succeeded bool) {
	fs.readyOnce.Do(func() {
		fs.isInitialized = succeeded
		if fs.readyCh != nil {
			close(fs.readyCh)
		}
	})
}

// Close is called automatically when the client is closed.
func (fs *fileDataSource) Close() (err error) {
	fs.closeOnce.Do(func() {
		close(fs.closeReloaderCh)
	})
	return nil
}

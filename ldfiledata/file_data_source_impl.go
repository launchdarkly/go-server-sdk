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
	reloader              *filedata.Reloader
	loggers               ldlog.Loggers
	isInitialized         bool
	readyCh               chan<- struct{}
	readyOnce             sync.Once
	closeOnce             sync.Once
	// closeReloaderCh is created up front rather than when the reloader starts, so that
	// Close never races with Start assigning it.
	closeReloaderCh chan struct{}
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

	// Debouncing and automatic retries only matter when something can trigger further
	// reloads; a source configured without a reloader loads exactly once. Like
	// closeReloaderCh, the Reloader is created up front so that Close never races an
	// assignment made in Start.
	var debounceDelay, retryDelay time.Duration
	if reloaderFactory != nil {
		debounceDelay = filedata.DefaultDebounceDelay
		retryDelay = filedata.DefaultRetryDelay
	}
	fs.reloader = filedata.NewReloader(filedata.ReloaderConfig{
		Paths:                 fs.absFilePaths,
		DuplicateKeysHandling: filedata.DuplicateKeysHandling(fs.duplicateKeysHandling),
		Loggers:               fs.loggers,
		Apply:                 fs.applyData,
		OnError:               fs.handleError,
		DebounceDelay:         debounceDelay,
		RetryDelay:            retryDelay,
		SkipUnchanged:         true,
	})
	return fs, nil
}

func (fs *fileDataSource) IsInitialized() bool {
	return fs.isInitialized
}

func (fs *fileDataSource) Start(closeWhenReady chan<- struct{}) {
	fs.readyCh = closeWhenReady
	fs.reloader.ReloadNow()

	// If there is no reloader, then we signal readiness immediately regardless of whether the
	// data load succeeded or failed.
	if fs.reloaderFactory == nil {
		fs.signalStartComplete(fs.isInitialized)
		return
	}

	// If there is a reloader, and if we haven't yet successfully loaded data, then the
	// readiness signal will happen the first time we do get valid data (in applyData).
	err := fs.reloaderFactory(fs.absFilePaths, fs.loggers, fs.reloader.Trigger, fs.closeReloaderCh)
	if err != nil {
		fs.loggers.Errorf("Unable to start reloader: %s\n", err)
	}
}

func (fs *fileDataSource) applyData(merged filedata.MergeResult) {
	storeData := []ldstoretypes.Collection{
		{Kind: datakinds.Features, Items: merged.Flags},
		{Kind: datakinds.Segments, Items: merged.Segments},
	}
	if fs.dataSourceUpdates.Init(storeData) {
		fs.signalStartComplete(true)
		fs.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
	}
}

func (fs *fileDataSource) handleError(err error) {
	fs.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateInterrupted,
		interfaces.DataSourceErrorInfo{
			Kind:    interfaces.DataSourceErrorKindInvalidData,
			Message: err.Error(),
			Time:    time.Now(),
		})
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
		fs.reloader.Close()
	})
	return nil
}

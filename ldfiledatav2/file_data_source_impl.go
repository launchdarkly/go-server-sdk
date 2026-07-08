package ldfiledatav2

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/filedata"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

type fileDataSource struct {
	changeSetBroadcaster *internal.Broadcaster[subsystems.ChangeSet]
	statusBroadcaster    *internal.Broadcaster[interfaces.DataSynchronizerStatus]
	// NOTE: this is not really used anymore because file data sources at this
	// moment will not report a selector. It is atomic because loads can happen
	// concurrently from Fetch and from the reloader.
	version atomic.Int64

	absFilePaths          []string
	duplicateKeysHandling DuplicateKeysHandling
	reloaderFactory       ReloaderFactory
	reloader              *filedata.Reloader
	loggers               ldlog.Loggers
	// closeReloaderCh is created up front rather than when the reloader starts, so that
	// Close never races with Sync assigning it.
	closeReloaderCh chan struct{}

	closed      atomic.Bool
	syncStarted atomic.Bool
	quit        chan struct{}
}

func newFileDataSourceImpl(
	context subsystems.ClientContext,
	filePaths []string,
	duplicateKeysHandling DuplicateKeysHandling,
	reloaderFactory ReloaderFactory,
) (subsystems.DataSynchronizer, error) {
	abs, err := filedata.AbsFilePaths(filePaths)
	if err != nil {
		// COVERAGE: there's no reliable cross-platform way to simulate an invalid path in unit tests
		return nil, err
	}

	fs := &fileDataSource{
		changeSetBroadcaster:  internal.NewBroadcaster[subsystems.ChangeSet](),
		statusBroadcaster:     internal.NewBroadcaster[interfaces.DataSynchronizerStatus](),
		absFilePaths:          abs,
		duplicateKeysHandling: duplicateKeysHandling,
		reloaderFactory:       reloaderFactory,
		loggers:               context.GetLogging().Loggers,
		closeReloaderCh:       make(chan struct{}),
		quit:                  make(chan struct{}),
	}
	fs.loggers.SetPrefix("FileDataSource:")

	// Debouncing and automatic retries only matter when something can trigger further
	// reloads; a source configured without a reloader loads exactly once. Like
	// closeReloaderCh, the Reloader is created up front so that Close never races an
	// assignment made in Sync, and a repeated Sync cannot orphan an earlier instance.
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

func (fs *fileDataSource) Name() string {
	return "FileDataSynchronizer"
}

func (fs *fileDataSource) Sync(ds subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	// Sync starts the reloader and the file watcher and registers listeners for this
	// call's result loop, so it can run at most once: a repeat would accumulate another
	// watcher and listener pair feeding nothing. The rejection channel is buffered so an
	// unread rejection cannot block.
	if fs.closed.Load() || !fs.syncStarted.CompareAndSwap(false, true) {
		rejected := make(chan subsystems.DataSynchronizerResult, 1)
		rejected <- subsystems.DataSynchronizerResult{State: interfaces.DataSourceStateOff}
		close(rejected)
		return rejected
	}

	resultChan := make(chan subsystems.DataSynchronizerResult)

	changeSetChan := fs.changeSetBroadcaster.AddListener()
	statusChan := fs.statusBroadcaster.AddListener()

	result := subsystems.DataSynchronizerResult{
		State: interfaces.DataSourceStateInitializing,
	}

	fs.reloader.ReloadNow()

	if fs.reloaderFactory != nil {
		err := fs.reloaderFactory(fs.absFilePaths, fs.loggers, fs.reloader.Trigger, fs.closeReloaderCh)
		if err != nil {
			fs.loggers.Errorf("Unable to start reloader: %s\n", err)
			result.State = interfaces.DataSourceStateOff
			resultChan <- result
			close(resultChan)
			return resultChan
		}
	}

	go func() {
		defer close(resultChan)

		for {
			select {
			case <-fs.quit:
				return
			case changeSet, ok := <-changeSetChan:
				if !ok {
					return
				}

				result.ChangeSet = &changeSet
				result.State = interfaces.DataSourceStateValid
				result.Error = interfaces.DataSourceErrorInfo{}
				resultChan <- result
			case statusChange, ok := <-statusChan:
				if !ok {
					return
				}

				if statusChange.State != interfaces.DataSourceStateValid {
					result.ChangeSet = nil
				}
				result.State = statusChange.State
				result.Error = statusChange.Error
				resultChan <- result
			}
		}
	}()

	return resultChan
}

func (fs *fileDataSource) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, bool, error) {
	docs := make([]filedata.Document, 0, len(fs.absFilePaths))
	for _, path := range fs.absFilePaths {
		doc, err := filedata.ReadFile(path)
		if err != nil {
			return nil, false, &filedata.ReadError{Err: err, Path: path}
		}
		docs = append(docs, doc)
	}
	merged, err := filedata.Merge(filedata.DuplicateKeysHandling(fs.duplicateKeysHandling), docs...)
	if err != nil {
		return nil, false, err
	}
	changeSet, err := fs.makeChangeSet(merged)
	if err != nil {
		return nil, false, err
	}
	return &subsystems.Basis{
		ChangeSet: *changeSet,
		Persist:   false,
	}, false, nil
}

func (fs *fileDataSource) applyData(merged filedata.MergeResult) {
	changeSet, err := fs.makeChangeSet(merged)
	if err == nil {
		fs.changeSetBroadcaster.Broadcast(*changeSet)
	} else {
		fs.handleError(err)
	}
}

func (fs *fileDataSource) handleError(err error) {
	errorKind := interfaces.DataSourceErrorKindInvalidData
	var readErr *filedata.ReadError
	if errors.As(err, &readErr) {
		errorKind = interfaces.DataSourceErrorKindUnknown
	}
	fs.statusBroadcaster.Broadcast(interfaces.DataSynchronizerStatus{
		State: interfaces.DataSourceStateInterrupted,
		Error: interfaces.DataSourceErrorInfo{
			Kind:       errorKind,
			StatusCode: 0,
			Message:    err.Error(),
			Time:       time.Time{},
		},
		FallbackToFDv1: false,
	})
}

// makeChangeSet expresses a merged file data set as a full-transfer change set.
func (fs *fileDataSource) makeChangeSet(merged filedata.MergeResult) (*subsystems.ChangeSet, error) {
	intent := subsystems.ServerIntent{
		Payload: subsystems.Payload{
			ID:     "",
			Target: int(fs.version.Add(1)),
			Code:   subsystems.IntentTransferFull,
			Reason: "payload-missing",
		},
	}

	collections := make([]ldstoretypes.Collection, 0, 2)
	if len(merged.Flags) > 0 {
		collections = append(collections, ldstoretypes.Collection{
			Kind:  ldstoreimpl.Features(),
			Items: merged.Flags,
		})
	}
	if len(merged.Segments) > 0 {
		collections = append(collections, ldstoretypes.Collection{
			Kind:  ldstoreimpl.Segments(),
			Items: merged.Segments,
		})
	}

	// File data source will not have a selector for now.
	// NOTE: If we start supporting FDv2 data from file then this statement might change.
	// When that happens we will construct the selector the same way that we construct it
	// in the FDv2 polling data source.
	return subsystems.NewChangeSetFromCollections(intent, subsystems.NoSelector(), collections)
}

// Close is called automatically when the client is closed.
func (fs *fileDataSource) Close() (err error) {
	if swapped := fs.closed.CompareAndSwap(false, true); swapped {
		close(fs.quit)
		close(fs.closeReloaderCh)
		fs.reloader.Close()
		return nil // already closed
	}

	return nil
}

package ldfiledatav2

import (
	"context"
	"errors"
	"fmt"
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
	// moment will not report a selector.
	version int

	absFilePaths          []string
	duplicateKeysHandling DuplicateKeysHandling
	reloaderFactory       ReloaderFactory
	loggers               ldlog.Loggers
	closeReloaderCh       chan struct{}

	closed atomic.Bool
	quit   chan struct{}
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
		quit:                  make(chan struct{}),
	}
	fs.loggers.SetPrefix("FileDataSource:")
	return fs, nil
}

func (fs *fileDataSource) Name() string {
	return "FileDataSynchronizer"
}

func (fs *fileDataSource) Sync(ds subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	resultChan := make(chan subsystems.DataSynchronizerResult)

	changeSetChan := fs.changeSetBroadcaster.AddListener()
	statusChan := fs.statusBroadcaster.AddListener()

	result := subsystems.DataSynchronizerResult{
		State: interfaces.DataSourceStateInitializing,
	}

	if fs.closed.Load() {
		result.State = interfaces.DataSourceStateOff
		resultChan <- result
		close(resultChan)
		return resultChan
	}

	fs.reload()
	if fs.reloaderFactory != nil {
		fs.closeReloaderCh = make(chan struct{})
		err := fs.reloaderFactory(fs.absFilePaths, fs.loggers, fs.reload, fs.closeReloaderCh)
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
	changeSet, err := fs.load()
	if err != nil {
		return nil, false, err
	}
	return &subsystems.Basis{
		ChangeSet: *changeSet,
		Persist:   false,
	}, false, nil
}

// Reload tells the data source to immediately attempt to reread all of the configured source files
// and update the feature flag state. If any file cannot be loaded or parsed, the flag state will not
// be modified.
func (fs *fileDataSource) reload() {
	if fs.closeReloaderCh != nil {
		fs.loggers.Info("Reloading flag data after detecting a change")
	}

	changeSet, err := fs.load()
	if err == nil {
		fs.changeSetBroadcaster.Broadcast(*changeSet)
	} else {
		fs.loggers.Errorf("Unable to load flags: %s", err)
		errorKind := interfaces.DataSourceErrorKindInvalidData
		var readErr *fileReadError
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
}

// fileReadError distinguishes a failure to read or parse one of the source files from a
// failure to merge their contents.
type fileReadError struct {
	err  error
	path string
}

func (e *fileReadError) Error() string {
	return fmt.Sprintf("%s [%s]", e.err, e.path)
}

func (e *fileReadError) Unwrap() error {
	return e.err
}

// load synchronously reads and merges all of the configured source files, returning the
// result as a full-transfer change set.
func (fs *fileDataSource) load() (*subsystems.ChangeSet, error) {
	docs := make([]filedata.Document, 0)
	for _, path := range fs.absFilePaths {
		doc, err := filedata.ReadFile(path)
		if err != nil {
			return nil, &fileReadError{err: err, path: path}
		}
		docs = append(docs, doc)
	}

	merged, err := filedata.Merge(filedata.DuplicateKeysHandling(fs.duplicateKeysHandling), docs...)
	if err != nil {
		return nil, err
	}

	fs.version++
	intent := subsystems.ServerIntent{
		Payload: subsystems.Payload{
			ID:     "",
			Target: fs.version,
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

		if fs.closeReloaderCh != nil {
			close(fs.closeReloaderCh)
		}
		return nil // already closed
	}

	return nil
}

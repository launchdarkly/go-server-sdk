package ldfiledatav2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"gopkg.in/ghodss/yaml.v1"
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
	abs, err := absFilePaths(filePaths)
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

func (fs *fileDataSource) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, error) {
	changeSetChan := fs.changeSetBroadcaster.AddListener()
	statusChan := fs.statusBroadcaster.AddListener()

	var err error
	basis := &subsystems.Basis{
		ChangeSet: subsystems.ChangeSet{},
		Persist:   false,
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		fs.reload()

		defer wg.Done()

		select {
		case changeSet, ok := <-changeSetChan:
			if !ok {
				return
			}
			basis.ChangeSet = changeSet
			return
		case statusChange, ok := <-statusChan:
			if !ok {
				return
			}

			if statusChange.State != interfaces.DataSourceStateValid {
				err = errors.New("data source did not receive change set")
				return
			}
		default:
			return
		}
	}()

	wg.Wait()

	return basis, err
}

// Reload tells the data source to immediately attempt to reread all of the configured source files
// and update the feature flag state. If any file cannot be loaded or parsed, the flag state will not
// be modified.
func (fs *fileDataSource) reload() {
	if fs.closeReloaderCh != nil {
		fs.loggers.Info("Reloading flag data after detecting a change")
	}

	filesData := make([]fileData, 0)
	for _, path := range fs.absFilePaths {
		data, err := readFile(path)
		if err == nil {
			filesData = append(filesData, data)
		} else {
			fs.loggers.Errorf("Unable to load flags: %s [%s]", err, path)
			fs.statusBroadcaster.Broadcast(interfaces.DataSynchronizerStatus{
				State: interfaces.DataSourceStateInterrupted,
				Error: interfaces.DataSourceErrorInfo{
					Kind:       interfaces.DataSourceErrorKindUnknown,
					StatusCode: 0,
					Message:    err.Error(),
					Time:       time.Time{},
				},
			})
			return
		}
	}

	fs.version++
	changeSet, err := mergeFileData(fs.duplicateKeysHandling, fs.version, filesData...)

	if err == nil {
		fs.changeSetBroadcaster.Broadcast(*changeSet)
	} else {
		fs.statusBroadcaster.Broadcast(interfaces.DataSynchronizerStatus{
			State: interfaces.DataSourceStateInterrupted,
			Error: interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindInvalidData,
				StatusCode: 0,
				Message:    err.Error(),
				Time:       time.Time{},
			},
			RevertToFDv1: false,
		})
		fs.loggers.Error(err)
	}
}

func absFilePaths(paths []string) ([]string, error) {
	absPaths := make([]string, 0)
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			// COVERAGE: there's no reliable cross-platform way to simulate an invalid path in unit tests
			return nil, fmt.Errorf("unable to determine absolute path for '%s'", p)
		}
		absPaths = append(absPaths, absPath)
	}
	return absPaths, nil
}

type fileData struct {
	Flags      *map[string]ldmodel.FeatureFlag
	FlagValues *map[string]ldvalue.Value
	Segments   *map[string]ldmodel.Segment
}

func insertData(
	seenKeys map[subsystems.ObjectKind]map[string]bool,
	builder *subsystems.ChangeSetBuilder,
	objectKind subsystems.ObjectKind,
	key string,
	version int,
	data ldstoretypes.ItemDescriptor,
	duplicateKeysHandling DuplicateKeysHandling,
) error {
	if _, exists := seenKeys[objectKind][key]; exists {
		switch duplicateKeysHandling {
		case DuplicateKeysIgnoreAllButFirst:
			return nil
		default:
			return fmt.Errorf("%s '%s' is specified by multiple files", objectKind, key)
		}
	}

	json, err := json.Marshal(data.Item)
	if err != nil {
		return fmt.Errorf("error serializing %s '%s': %w", objectKind, key, err)
	}

	builder.AddPut(objectKind, key, version, json)
	seenKeys[objectKind][key] = true

	return nil
}

func readFile(path string) (fileData, error) {
	var data fileData
	var rawData []byte
	var err error
	if rawData, err = os.ReadFile(path); err != nil { //nolint:gosec // G304: ok to read file into variable
		return data, fmt.Errorf("unable to read file: %s", err)
	}
	if detectJSON(rawData) {
		err = json.Unmarshal(rawData, &data)
	} else {
		err = yaml.Unmarshal(rawData, &data)
	}
	if err != nil {
		err = fmt.Errorf("error parsing file: %s", err)
	}
	return data, err
}

func detectJSON(rawData []byte) bool {
	// A valid JSON file for our purposes must be an object, i.e. it must start with '{'
	return strings.HasPrefix(strings.TrimLeftFunc(string(rawData), unicode.IsSpace), "{")
}

func mergeFileData(
	duplicateKeysHandling DuplicateKeysHandling,
	version int,
	allFileData ...fileData,
) (*subsystems.ChangeSet, error) {
	builder := subsystems.NewChangeSetBuilder()
	err := builder.Start(subsystems.ServerIntent{
		Payload: subsystems.Payload{
			ID:     "",
			Target: version,
			Code:   subsystems.IntentTransferFull,
			Reason: "payload-missing",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error starting change set: %w", err)
	}

	seenKeys := map[subsystems.ObjectKind]map[string]bool{
		subsystems.FlagKind:    {},
		subsystems.SegmentKind: {},
	}
	for _, d := range allFileData {
		if d.Flags != nil {
			for key, f := range *d.Flags {
				data := ldstoretypes.ItemDescriptor{Version: f.Version, Item: &f}
				err := insertData(seenKeys, builder, subsystems.FlagKind, key, f.Version, data, duplicateKeysHandling)
				if err != nil {
					return nil, err
				}
			}
		}
		if d.FlagValues != nil {
			for key, value := range *d.FlagValues {
				flag := makeFlagWithValue(key, value)
				data := ldstoretypes.ItemDescriptor{Version: flag.Version, Item: flag}
				err := insertData(seenKeys, builder, subsystems.FlagKind, key, flag.Version, data, duplicateKeysHandling)
				if err != nil {
					return nil, err
				}
			}
		}
		if d.Segments != nil {
			for key, s := range *d.Segments {
				data := ldstoretypes.ItemDescriptor{Version: s.Version, Item: &s}
				err := insertData(seenKeys, builder, subsystems.SegmentKind, key, s.Version, data, duplicateKeysHandling)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// File data source will not have a selector for now.
	// NOTE: If we start supporting FDv2 data from file then this statement might change.
	// When that happens we will construct the selector the same way that we construct it
	// in the FDv2 polling data source.
	return builder.Finish(subsystems.NoSelector())
}

func makeFlagWithValue(key string, v interface{}) *ldmodel.FeatureFlag {
	flag := ldbuilders.NewFlagBuilder(key).SingleVariation(ldvalue.CopyArbitraryValue(v)).Build()
	return &flag
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

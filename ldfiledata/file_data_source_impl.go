package ldfiledata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"gopkg.in/ghodss/yaml.v1"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
)

type fileDataSource struct {
	dataSourceUpdates interfaces.DataSourceUpdates
	absFilePaths      []string
	reloaderFactory   ReloaderFactory
	loggers           ldlog.Loggers
	isInitialized     bool
	readyCh           chan<- struct{}
	readyOnce         sync.Once
	closeOnce         sync.Once
	closeReloaderCh   chan struct{}
}

func newFileDataSourceImpl(
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
	filePaths []string,
	reloaderFactory ReloaderFactory,
) (interfaces.DataSource, error) {
	abs, err := absFilePaths(filePaths)
	if err != nil {
		// COVERAGE: there's no reliable cross-platform way to simulate an invalid path in unit tests
		return nil, err
	}

	fs := &fileDataSource{
		dataSourceUpdates: dataSourceUpdates,
		absFilePaths:      abs,
		reloaderFactory:   reloaderFactory,
		loggers:           context.GetLogging().GetLoggers(),
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
	fs.closeReloaderCh = make(chan struct{})
	err := fs.reloaderFactory(fs.absFilePaths, fs.loggers, fs.reload, fs.closeReloaderCh)
	if err != nil {
		fs.loggers.Errorf("Unable to start reloader: %s\n", err)
	}
}

// Reload tells the data source to immediately attempt to reread all of the configured source files
// and update the feature flag state. If any file cannot be loaded or parsed, the flag state will not
// be modified.
func (fs *fileDataSource) reload() {
	filesData := make([]fileData, 0)
	for _, path := range fs.absFilePaths {
		data, err := readFile(path)
		if err == nil {
			filesData = append(filesData, data)
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
	storeData, err := mergeFileData(filesData...)
	if err == nil {
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
	all map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor,
	kind ldstoretypes.DataKind,
	key string,
	data ldstoretypes.ItemDescriptor,
) error {
	if _, exists := all[kind][key]; exists {
		return fmt.Errorf("%s '%s' is specified by multiple files", kind, key)
	}
	all[kind][key] = data
	return nil
}

func readFile(path string) (fileData, error) {
	var data fileData
	var rawData []byte
	var err error
	if rawData, err = ioutil.ReadFile(path); err != nil { // nolint:gosec // G304: ok to read file into variable
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

func mergeFileData(allFileData ...fileData) ([]ldstoretypes.Collection, error) {
	all := map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor{
		datakinds.Features: {},
		datakinds.Segments: {},
	}
	for _, d := range allFileData {
		if d.Flags != nil {
			for key, f := range *d.Flags {
				ff := f
				data := ldstoretypes.ItemDescriptor{Version: f.Version, Item: &ff}
				if err := insertData(all, datakinds.Features, key, data); err != nil {
					return nil, err
				}
			}
		}
		if d.FlagValues != nil {
			for key, value := range *d.FlagValues {
				flag := makeFlagWithValue(key, value)
				data := ldstoretypes.ItemDescriptor{Version: flag.Version, Item: flag}
				if err := insertData(all, datakinds.Features, key, data); err != nil {
					return nil, err
				}
			}
		}
		if d.Segments != nil {
			for key, s := range *d.Segments {
				ss := s
				data := ldstoretypes.ItemDescriptor{Version: s.Version, Item: &ss}
				if err := insertData(all, datakinds.Segments, key, data); err != nil {
					return nil, err
				}
			}
		}
	}
	ret := []ldstoretypes.Collection{}
	for kind, itemsMap := range all {
		items := make([]ldstoretypes.KeyedItemDescriptor, 0, len(itemsMap))
		for k, v := range itemsMap {
			items = append(items, ldstoretypes.KeyedItemDescriptor{Key: k, Item: v})
		}
		ret = append(ret, ldstoretypes.Collection{Kind: kind, Items: items})
	}
	return ret, nil
}

func makeFlagWithValue(key string, v interface{}) *ldmodel.FeatureFlag {
	flag := ldbuilders.NewFlagBuilder(key).SingleVariation(ldvalue.CopyArbitraryValue(v)).Build()
	return &flag
}

// Close is called automatically when the client is closed.
func (fs *fileDataSource) Close() (err error) {
	fs.closeOnce.Do(func() {
		if fs.closeReloaderCh != nil {
			close(fs.closeReloaderCh)
		}
	})
	return nil
}

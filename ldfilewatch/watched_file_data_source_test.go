package ldfilewatch

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v6/ldfiledata"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

type fileDataSourceTestParams struct {
	dataSource     subsystems.DataSource
	updates        *mocks.MockDataSourceUpdates
	mockLog        *ldlogtest.MockLog
	closeWhenReady chan struct{}
}

func (p fileDataSourceTestParams) waitForStart() {
	p.dataSource.Start(p.closeWhenReady)
	<-p.closeWhenReady
}

func withFileDataSourceTestParams(
	factory subsystems.ComponentConfigurer[subsystems.DataSource],
	action func(fileDataSourceTestParams),
) {
	p := fileDataSourceTestParams{}
	p.closeWhenReady = make(chan struct{})
	p.mockLog = ldlogtest.NewMockLog()
	testContext := sharedtest.NewTestContext("", nil, &subsystems.LoggingConfiguration{Loggers: p.mockLog.Loggers})
	store, _ := ldcomponents.InMemoryDataStore().Build(testContext)
	p.updates = mocks.NewMockDataSourceUpdates(store)
	testContext.DataSourceUpdateSink = p.updates
	dataSource, err := factory.Build(testContext)
	if err != nil {
		panic(err)
	}
	defer dataSource.Close()
	p.dataSource = dataSource
	action(p)
}

func withTempDir(action func(dirPath string)) {
	// We should put the temp files in their own directory; otherwise, we might be running a file watcher over
	// all of /tmp, which is not a great idea
	path, err := os.MkdirTemp("", "watched-file-data-source-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(path)
	action(path)
}

func makeTempFile(dirPath, initialText string) string {
	f, err := os.CreateTemp(dirPath, "file-source-test")
	if err != nil {
		panic(err)
	}
	f.WriteString(initialText)
	err = f.Close()
	if err != nil {
		panic(err)
	}
	return f.Name()
}

func replaceFileContents(filename string, text string) {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	f.WriteString(text)
	err = f.Sync()
	if err != nil {
		panic(err)
	}
	f.Close()
}

func requireTrueWithinDuration(t *testing.T, maxTime time.Duration, test func() bool) {
	t.Helper()
	deadline := time.Now().Add(maxTime)
	for {
		if time.Now().After(deadline) {
			require.FailNowf(t, "Did not see expected change", "waited %v", maxTime)
		}
		if test() {
			return
		}
		time.Sleep(time.Millisecond * 100)
	}
}

func hasFlag(t *testing.T, store subsystems.DataStore, key string, test func(ldmodel.FeatureFlag) bool) bool {
	flagItem, err := store.Get(datakinds.Features, key)
	if assert.NoError(t, err) && flagItem.Item != nil {
		return test(*(flagItem.Item.(*ldmodel.FeatureFlag)))
	}
	return false
}

func TestNewWatchedFileDataSource(t *testing.T) {
	withTempDir(func(tempDir string) {
		filename := makeTempFile(tempDir, `
---
flags: bad
`)
		defer os.Remove(filename)

		factory := ldfiledata.DataSource().
			FilePaths(filename).
			Reloader(WatchFiles)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.dataSource.Start(p.closeWhenReady)

			// Create the flags file after we start
			time.Sleep(time.Second)
			replaceFileContents(filename, `
---
flags:
  my-flag:
    "on": true
`)

			<-p.closeWhenReady

			// Don't use waitForExpectedChange here because the expectation is that as soon as the dataSource
			// reports being ready (which it will only do once we've given it a valid file), the flag should
			// be available immediately.
			assert.True(t, hasFlag(t, p.updates.DataStore, "my-flag", func(f ldmodel.FeatureFlag) bool {
				return f.On
			}))
			assert.True(t, p.dataSource.IsInitialized())

			// Update the file
			replaceFileContents(filename, `
---
flags:
  my-flag:
    "on": false
`)

			requireTrueWithinDuration(t, time.Second, func() bool {
				return hasFlag(t, p.updates.DataStore, "my-flag", func(f ldmodel.FeatureFlag) bool {
					return !f.On
				})
			})
			p.mockLog.AssertMessageMatch(t, true, ldlog.Info, "Reloading flag data after detecting a change")
		})
	})
}

// File need not exist when the dataSource is started
func TestNewWatchedFileMissing(t *testing.T) {
	withTempDir(func(tempDir string) {
		filename := makeTempFile(tempDir, "")
		require.NoError(t, os.Remove(filename))
		defer os.Remove(filename)

		factory := ldfiledata.DataSource().
			FilePaths(filename).
			Reloader(WatchFiles)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.dataSource.Start(p.closeWhenReady)

			time.Sleep(time.Second)
			replaceFileContents(filename, `
---
flags:
  my-flag:
    "on": true
`)

			<-p.closeWhenReady

			requireTrueWithinDuration(t, time.Second, func() bool {
				return hasFlag(t, p.updates.DataStore, "my-flag", func(f ldmodel.FeatureFlag) bool {
					return f.On
				})
			})
			assert.True(t, p.dataSource.IsInitialized())
		})
	})
}

// Directory needn't exist when the dataSource is started
func TestNewWatchedDirectoryMissing(t *testing.T) {
	withTempDir(func(tempDir string) {
		tempDir, err := os.MkdirTemp("", "file-source-test")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir)

		dirPath := path.Join(tempDir, "test")
		filePath := path.Join(dirPath, "flags.yml")

		factory := ldfiledata.DataSource().
			FilePaths(filePath).
			Reloader(WatchFiles)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.dataSource.Start(p.closeWhenReady)

			time.Sleep(time.Second)
			err = os.Mkdir(dirPath, 0700)
			require.NoError(t, err)

			time.Sleep(time.Second)
			replaceFileContents(filePath, `
---
flags:
  my-flag:
    "on": true
`)

			<-p.closeWhenReady

			requireTrueWithinDuration(t, time.Second*2, func() bool {
				return hasFlag(t, p.updates.DataStore, "my-flag", func(f ldmodel.FeatureFlag) bool {
					return f.On
				})
			})
			assert.True(t, p.dataSource.IsInitialized())
		})
	})
}

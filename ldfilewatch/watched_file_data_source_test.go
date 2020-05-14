package ldfilewatch

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldfiledata"
)

func makeTempFile(t *testing.T, initialText string) string {
	f, err := ioutil.TempFile("", "file-source-test")
	require.NoError(t, err)
	f.WriteString(initialText)
	require.NoError(t, f.Close())
	return f.Name()
}

func testContext() interfaces.ClientContext {
	return interfaces.NewClientContext("", nil, nil, ldlog.NewDisabledLoggers())
}

func makeDataStore() interfaces.DataStore {
	store, _ := ldcomponents.InMemoryDataStore().CreateDataStore(testContext())
	return store
}

func replaceFileContents(t *testing.T, filename string, text string) {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0600)
	require.NoError(t, err)
	f.WriteString(text)
	require.NoError(t, f.Sync())
	f.Close()
}

func requireTrueWithinDuration(t *testing.T, maxTime time.Duration, test func() bool) {
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

func hasFlag(t *testing.T, store interfaces.DataStore, key string, test func(ldmodel.FeatureFlag) bool) bool {
	flagItem, err := store.Get(interfaces.DataKindFeatures(), key)
	if assert.NoError(t, err) && flagItem.Item != nil {
		return test(*(flagItem.Item.(*ldmodel.FeatureFlag)))
	}
	return false
}

func TestNewWatchedFileDataSource(t *testing.T) {
	filename := makeTempFile(t, `
---
flags: bad
`)
	defer os.Remove(filename)

	store := makeDataStore()

	factory := ldfiledata.NewFileDataSourceFactory(
		ldfiledata.FilePaths(filename),
		ldfiledata.UseReloader(WatchFiles))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	defer dataSource.Close()

	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)

	// Create the flags file after we start
	time.Sleep(time.Second)
	replaceFileContents(t, filename, `
---
flags:
  my-flag:
    "on": true
`)

	<-closeWhenReady

	// Don't use waitForExpectedChange here because the expectation is that as soon as the dataSource
	// reports being ready (which it will only do once we've given it a valid file), the flag should
	// be available immediately.
	assert.True(t, hasFlag(t, store, "my-flag", func(f ldmodel.FeatureFlag) bool {
		return f.On
	}))
	assert.True(t, dataSource.Initialized())

	// Update the file
	replaceFileContents(t, filename, `
---
flags:
  my-flag:
    "on": false
`)

	requireTrueWithinDuration(t, time.Second, func() bool {
		return hasFlag(t, store, "my-flag", func(f ldmodel.FeatureFlag) bool {
			return !f.On
		})
	})
}

// File need not exist when the dataSource is started
func TestNewWatchedFileMissing(t *testing.T) {
	filename := makeTempFile(t, "")
	require.NoError(t, os.Remove(filename))
	defer os.Remove(filename)

	store := makeDataStore()

	factory := ldfiledata.NewFileDataSourceFactory(
		ldfiledata.FilePaths(filename),
		ldfiledata.UseReloader(WatchFiles))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	defer dataSource.Close()

	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)

	time.Sleep(time.Second)
	replaceFileContents(t, filename, `
---
flags:
  my-flag:
    "on": true
`)

	<-closeWhenReady

	requireTrueWithinDuration(t, time.Second, func() bool {
		return hasFlag(t, store, "my-flag", func(f ldmodel.FeatureFlag) bool {
			return f.On
		})
	})
	assert.True(t, dataSource.Initialized())
}

// Directory needn't exist when the dataSource is started
func TestNewWatchedDirectoryMissing(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "file-source-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)
	store := makeDataStore()

	dirPath := path.Join(tempDir, "test")
	filePath := path.Join(dirPath, "flags.yml")

	factory := ldfiledata.NewFileDataSourceFactory(
		ldfiledata.FilePaths(filePath),
		ldfiledata.UseReloader(WatchFiles))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	defer dataSource.Close()

	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)

	time.Sleep(time.Second)
	err = os.Mkdir(dirPath, 0700)
	require.NoError(t, err)

	time.Sleep(time.Second)
	replaceFileContents(t, filePath, `
---
flags:
  my-flag:
    "on": true
`)

	<-closeWhenReady

	requireTrueWithinDuration(t, time.Second*2, func() bool {
		return hasFlag(t, store, "my-flag", func(f ldmodel.FeatureFlag) bool {
			return f.On
		})
	})
	assert.True(t, dataSource.Initialized())
}

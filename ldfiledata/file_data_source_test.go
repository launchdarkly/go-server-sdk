package ldfiledata

import (
	"io/ioutil"
	"os"
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func X() {}

type fileDataSourceTestParams struct {
	dataSource     interfaces.DataSource
	updates        *sharedtest.MockDataSourceUpdates
	closeWhenReady chan struct{}
}

func (p fileDataSourceTestParams) waitForStart() {
	p.dataSource.Start(p.closeWhenReady)
	<-p.closeWhenReady
}

func withFileDataSourceTestParams(factory interfaces.DataSourceFactory, action func(fileDataSourceTestParams)) {
	p := fileDataSourceTestParams{}
	testContext := interfaces.NewClientContext("", nil, nil, sharedtest.TestLogging())
	store, _ := ldcomponents.InMemoryDataStore().CreateDataStore(testContext, nil)
	updates := sharedtest.NewMockDataSourceUpdates(store)
	dataSource, err := factory.CreateDataSource(testContext, updates)
	if err != nil {
		panic(err)
	}
	defer dataSource.Close()
	p.dataSource = dataSource
	action(fileDataSourceTestParams{dataSource, updates, make(chan struct{})})
}

func withTempFileContaining(initialText string, action func(filename string)) {
	f, err := ioutil.TempFile("", "file-dataSource-test")
	if err != nil {
		panic(err)
	}
	f.WriteString(initialText)
	err = f.Close()
	if err != nil {
		panic(err)
	}
	filename := f.Name()
	defer os.Remove(filename)
	action(filename)
}

func TestNewFileDataSourceYaml(t *testing.T) {
	fileData := `
---
flags:
  my-flag:
    "on": true
segments:
  my-segment:
    rules: []
`
	withTempFileContaining(fileData, func(filename string) {
		factory := NewFileDataSourceFactory(FilePaths(filename))
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			flagItem, err := p.updates.DataStore.Get(interfaces.DataKindFeatures(), "my-flag")
			require.NoError(t, err)
			require.NotNil(t, flagItem.Item)
			assert.True(t, flagItem.Item.(*ldmodel.FeatureFlag).On)

			segmentItem, err := p.updates.DataStore.Get(interfaces.DataKindSegments(), "my-segment")
			require.NoError(t, err)
			require.NotNil(t, segmentItem.Item)
			assert.Empty(t, segmentItem.Item.(*ldmodel.Segment).Rules)
		})
	})
}

func TestNewFileDataSourceJson(t *testing.T) {
	withTempFileContaining(`{"flags": {"my-flag": {"on": true}}}`, func(filename string) {
		factory := NewFileDataSourceFactory(FilePaths(filename))
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			flagItem, err := p.updates.DataStore.Get(interfaces.DataKindFeatures(), "my-flag")
			require.NoError(t, err)
			require.NotNil(t, flagItem.Item)
			assert.True(t, flagItem.Item.(*ldmodel.FeatureFlag).On)
		})
	})
}

func TestStatusIsValidAfterSuccessfulLoad(t *testing.T) {
	withTempFileContaining(`{"flags": {"my-flag": {"on": true}}}`, func(filename string) {
		factory := NewFileDataSourceFactory(FilePaths(filename))
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid)
		})
	})
}

func TestNewFileDataSourceJsonWithTwoFiles(t *testing.T) {
	withTempFileContaining(`{"flags": {"my-flag1": {"on": true}}}`, func(filename1 string) {
		withTempFileContaining(`{"flags": {"my-flag2": {"on": true}}}`, func(filename2 string) {
			factory := NewFileDataSourceFactory(FilePaths(filename1, filename2))
			withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
				p.waitForStart()
				require.True(t, p.dataSource.IsInitialized())

				flagItem1, err := p.updates.DataStore.Get(interfaces.DataKindFeatures(), "my-flag1")
				require.NoError(t, err)
				require.NotNil(t, flagItem1.Item)
				assert.True(t, flagItem1.Item.(*ldmodel.FeatureFlag).On)

				flagItem2, err := p.updates.DataStore.Get(interfaces.DataKindFeatures(), "my-flag2")
				require.NoError(t, err)
				require.NotNil(t, flagItem2.Item)
				assert.True(t, flagItem2.Item.(*ldmodel.FeatureFlag).On)
			})
		})
	})
}

func TestNewFileDataSourceJsonWithTwoConflictingFiles(t *testing.T) {
	withTempFileContaining(`{"flags": {"my-flag1": {"on": true}}}`, func(filename1 string) {
		withTempFileContaining(`{"flags": {"my-flag1": {"on": true}}}`, func(filename2 string) {
			factory := NewFileDataSourceFactory(FilePaths(filename1, filename2))
			withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
				p.waitForStart()
				require.False(t, p.dataSource.IsInitialized())
			})
		})
	})
}

func TestNewFileDataSourceBadData(t *testing.T) {
	withTempFileContaining(`bad data`, func(filename string) {
		factory := NewFileDataSourceFactory(FilePaths(filename))
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.False(t, p.dataSource.IsInitialized())
		})
	})
}

func TestNewFileDataSourceMissingFile(t *testing.T) {
	withTempFileContaining("", func(filename string) {
		os.Remove(filename)

		factory := NewFileDataSourceFactory(FilePaths(filename))
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			assert.False(t, p.dataSource.IsInitialized())
		})
	})
}

func TestStatusIsInterruptedAfterUnsuccessfulLoad(t *testing.T) {
	withTempFileContaining(`bad data`, func(filename string) {
		factory := NewFileDataSourceFactory(FilePaths(filename))
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.False(t, p.dataSource.IsInitialized())

			p.updates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
		})
	})
}

func TestNewFileDataSourceYamlValues(t *testing.T) {
	fileData := `
---
flagValues:
  my-flag: true
`
	withTempFileContaining(fileData, func(filename string) {
		factory := NewFileDataSourceFactory(FilePaths(filename))
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())
			flagItem, err := p.updates.DataStore.Get(interfaces.DataKindFeatures(), "my-flag")
			require.NoError(t, err)
			require.NotNil(t, flagItem.Item)
			assert.Equal(t, []ldvalue.Value{ldvalue.Bool(true)}, flagItem.Item.(*ldmodel.FeatureFlag).Variations)
		})
	})
}

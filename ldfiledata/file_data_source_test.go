package ldfiledata

import (
	"errors"
	"os"
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fileDataSourceTestParams struct {
	dataSource     interfaces.DataSource
	updates        *sharedtest.MockDataSourceUpdates
	mockLog        *ldlogtest.MockLog
	closeWhenReady chan struct{}
}

func (p fileDataSourceTestParams) waitForStart() {
	p.dataSource.Start(p.closeWhenReady)
	<-p.closeWhenReady
}

func withFileDataSourceTestParams(factory interfaces.DataSourceFactory, action func(fileDataSourceTestParams)) {
	p := fileDataSourceTestParams{}
	mockLog := ldlogtest.NewMockLog()
	logConfig, _ := ldcomponents.Logging().Loggers(mockLog.Loggers).
		CreateLoggingConfiguration(interfaces.BasicConfiguration{})
	testContext := sharedtest.NewTestContext("", nil, logConfig)
	store, _ := ldcomponents.InMemoryDataStore().CreateDataStore(testContext, nil)
	updates := sharedtest.NewMockDataSourceUpdates(store)
	dataSource, err := factory.CreateDataSource(testContext, updates)
	if err != nil {
		panic(err)
	}
	defer dataSource.Close()
	p.dataSource = dataSource
	action(fileDataSourceTestParams{dataSource, updates, mockLog, make(chan struct{})})
}

func expectCreationError(t *testing.T, factory interfaces.DataSourceFactory) error {
	testContext := sharedtest.NewTestContext("", nil, sharedtest.TestLoggingConfig())
	store, _ := ldcomponents.InMemoryDataStore().CreateDataStore(testContext, nil)
	updates := sharedtest.NewMockDataSourceUpdates(store)
	dataSource, err := factory.CreateDataSource(testContext, updates)
	require.Error(t, err)
	require.Nil(t, dataSource)
	return err
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
	sharedtest.WithTempFileContaining([]byte(fileData), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			flagItem, err := p.updates.DataStore.Get(datakinds.Features, "my-flag")
			require.NoError(t, err)
			require.NotNil(t, flagItem.Item)
			assert.True(t, flagItem.Item.(*ldmodel.FeatureFlag).On)

			segmentItem, err := p.updates.DataStore.Get(datakinds.Segments, "my-segment")
			require.NoError(t, err)
			require.NotNil(t, segmentItem.Item)
			assert.Empty(t, segmentItem.Item.(*ldmodel.Segment).Rules)
		})
	})
}

func TestNewFileDataSourceJson(t *testing.T) {
	sharedtest.WithTempFileContaining([]byte(`{"flags": {"my-flag": {"on": true}}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			flagItem, err := p.updates.DataStore.Get(datakinds.Features, "my-flag")
			require.NoError(t, err)
			require.NotNil(t, flagItem.Item)
			assert.True(t, flagItem.Item.(*ldmodel.FeatureFlag).On)
		})
	})
}

func TestStatusIsValidAfterSuccessfulLoad(t *testing.T) {
	sharedtest.WithTempFileContaining([]byte(`{"flags": {"my-flag": {"on": true}}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid)
		})
	})
}

func TestNewFileDataSourceJsonWithTwoFiles(t *testing.T) {
	sharedtest.WithTempFileContaining([]byte(`{"flags": {"my-flag1": {"on": true}}}`), func(filename1 string) {
		sharedtest.WithTempFileContaining([]byte(`{"flags": {"my-flag2": {"on": true}}}`), func(filename2 string) {
			factory := DataSource().FilePaths(filename1, filename2)
			withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
				p.waitForStart()
				require.True(t, p.dataSource.IsInitialized())

				flagItem1, err := p.updates.DataStore.Get(datakinds.Features, "my-flag1")
				require.NoError(t, err)
				require.NotNil(t, flagItem1.Item)
				assert.True(t, flagItem1.Item.(*ldmodel.FeatureFlag).On)

				flagItem2, err := p.updates.DataStore.Get(datakinds.Features, "my-flag2")
				require.NoError(t, err)
				require.NotNil(t, flagItem2.Item)
				assert.True(t, flagItem2.Item.(*ldmodel.FeatureFlag).On)
			})
		})
	})
}

func TestNewFileDataSourceJsonWithTwoConflictingFiles(t *testing.T) {
	file1Data := `{"flags": {"flag1": {"on": true}, "flag2": {"on": true}}, "segments": {"segment1": {}}}`
	file2Data := `{"flags": {"flag2": {"on": true}}}`
	file3Data := `{"flagValues": {"flag2": true}}`
	file4Data := `{"segments": {"segment1": {}}}`

	sharedtest.WithTempFileContaining([]byte(file1Data), func(filename1 string) {
		for _, data := range []string{file2Data, file3Data, file4Data} {
			sharedtest.WithTempFileContaining([]byte(data), func(filename2 string) {
				factory := DataSource().FilePaths(filename1, filename2)
				withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
					p.waitForStart()
					require.False(t, p.dataSource.IsInitialized())
				})
			})
		}
	})
}

func TestNewFileDataSourceBadData(t *testing.T) {
	sharedtest.WithTempFileContaining([]byte(`bad data`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.False(t, p.dataSource.IsInitialized())
		})
	})
}

func TestNewFileDataSourceMissingFile(t *testing.T) {
	sharedtest.WithTempFileContaining([]byte{}, func(filename string) {
		os.Remove(filename)

		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			assert.False(t, p.dataSource.IsInitialized())
		})
	})
}

func TestStatusIsInterruptedAfterUnsuccessfulLoad(t *testing.T) {
	sharedtest.WithTempFileContaining([]byte(`bad data`), func(filename string) {
		factory := DataSource().FilePaths(filename)
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
	sharedtest.WithTempFileContaining([]byte(fileData), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())
			flagItem, err := p.updates.DataStore.Get(datakinds.Features, "my-flag")
			require.NoError(t, err)
			require.NotNil(t, flagItem.Item)
			assert.Equal(t, []ldvalue.Value{ldvalue.Bool(true)}, flagItem.Item.(*ldmodel.FeatureFlag).Variations)
		})
	})
}

func TestReloaderFailureDoesNotPreventStarting(t *testing.T) {
	e := errors.New("sorry")
	f := func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error {
		return e
	}
	factory := DataSource().Reloader(f)
	withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
		p.waitForStart()
		assert.True(t, p.dataSource.IsInitialized())
		assert.Len(t, p.mockLog.GetOutput(ldlog.Error), 1)
	})
}

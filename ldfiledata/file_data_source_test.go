package ldfiledata

import (
	"errors"
	"os"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	mockLog := ldlogtest.NewMockLog()
	testContext := sharedtest.NewTestContext("", nil, &subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
	store, _ := ldcomponents.InMemoryDataStore().Build(testContext)
	updates := mocks.NewMockDataSourceUpdates(store)
	testContext.DataSourceUpdateSink = updates
	dataSource, err := factory.Build(testContext)
	if err != nil {
		panic(err)
	}
	defer dataSource.Close()
	p.dataSource = dataSource
	action(fileDataSourceTestParams{dataSource, updates, mockLog, make(chan struct{})})
}

func expectCreationError(t *testing.T, factory subsystems.ComponentConfigurer[subsystems.DataSource]) error {
	testContext := sharedtest.NewTestContext("", nil, nil)
	store, _ := ldcomponents.InMemoryDataStore().Build(testContext)
	updates := mocks.NewMockDataSourceUpdates(store)
	testContext.DataSourceUpdateSink = updates
	dataSource, err := factory.Build(testContext)
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
	th.WithTempFileData([]byte(fileData), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			flag := requireFlag(t, p.updates.DataStore, "my-flag")
			assert.True(t, flag.On)

			segment := requireSegment(t, p.updates.DataStore, "my-segment")
			assert.Empty(t, segment.Rules)
		})
	})
}

func TestNewFileDataSourceJson(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true}}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			flag := requireFlag(t, p.updates.DataStore, "my-flag")
			assert.True(t, flag.On)
		})
	})
}

func TestStatusIsValidAfterSuccessfulLoad(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": true}}}`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid)
		})
	})
}

func TestNewFileDataSourceJsonWithTwoFiles(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag1": {"on": true}}}`), func(filename1 string) {
		th.WithTempFileData([]byte(`{"flags": {"my-flag2": {"on": true}}}`), func(filename2 string) {
			factory := DataSource().FilePaths(filename1, filename2)
			withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
				p.waitForStart()
				require.True(t, p.dataSource.IsInitialized())

				flag1 := requireFlag(t, p.updates.DataStore, "my-flag1")
				assert.True(t, flag1.On)

				flag2 := requireFlag(t, p.updates.DataStore, "my-flag2")
				assert.True(t, flag2.On)
			})
		})
	})
}

func TestNewFileDataSourceJsonWithTwoConflictingFiles(t *testing.T) {
	file1Data := `{"flags": {"flag1": {"on": true}, "flag2": {"on": true}}, "segments": {"segment1": {}}}`
	file2Data := `{"flags": {"flag2": {"on": true}}}`
	file3Data := `{"flagValues": {"flag2": true}}`
	file4Data := `{"segments": {"segment1": {}}}`

	th.WithTempFileData([]byte(file1Data), func(filename1 string) {
		for _, data := range []string{file2Data, file3Data, file4Data} {
			th.WithTempFileData([]byte(data), func(filename2 string) {
				factory := DataSource().FilePaths(filename1, filename2)
				withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
					p.waitForStart()
					require.False(t, p.dataSource.IsInitialized())

					p.mockLog.AssertMessageMatch(t, true, ldlog.Error, "specified by multiple files")
				})
			})
		}
	})
}

func TestDuplicateKeysHandlingCanSuppressErrors(t *testing.T) {
	file1Data := `{"flags": {"flag1": {"on": true}, "flag2": {"on": false}}, "segments": {"segment1": {}}}`
	file2Data := `{"flags": {"flag2": {"on": true}}}`

	th.WithTempFileData([]byte(file1Data), func(filename1 string) {
		th.WithTempFileData([]byte(file2Data), func(filename2 string) {
			factory := DataSource().FilePaths(filename1, filename2).
				DuplicateKeysHandling(DuplicateKeysIgnoreAllButFirst)
			withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
				p.waitForStart()
				require.True(t, p.dataSource.IsInitialized())

				flag2 := requireFlag(t, p.updates.DataStore, "flag2")
				assert.False(t, flag2.On)

				p.mockLog.AssertMessageMatch(t, false, ldlog.Error, "specified by multiple files")
			})
		})
	})
}

func TestNewFileDataSourceBadData(t *testing.T) {
	th.WithTempFileData([]byte(`bad data`), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.False(t, p.dataSource.IsInitialized())
		})
	})
}

func TestNewFileDataSourceMissingFile(t *testing.T) {
	th.WithTempFileData([]byte{}, func(filename string) {
		os.Remove(filename)

		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			assert.False(t, p.dataSource.IsInitialized())
		})
	})
}

func TestStatusIsInterruptedAfterUnsuccessfulLoad(t *testing.T) {
	th.WithTempFileData([]byte(`bad data`), func(filename string) {
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
	th.WithTempFileData([]byte(fileData), func(filename string) {
		factory := DataSource().FilePaths(filename)
		withFileDataSourceTestParams(factory, func(p fileDataSourceTestParams) {
			p.waitForStart()
			require.True(t, p.dataSource.IsInitialized())

			flag := requireFlag(t, p.updates.DataStore, "my-flag")
			assert.Equal(t, []ldvalue.Value{ldvalue.Bool(true)}, flag.Variations)
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

func requireFlag(t *testing.T, store subsystems.DataStore, key string) *ldmodel.FeatureFlag {
	item, err := store.Get(datakinds.Features, key)
	require.NoError(t, err)
	require.NotNil(t, item.Item)
	return item.Item.(*ldmodel.FeatureFlag)
}

func requireSegment(t *testing.T, store subsystems.DataStore, key string) *ldmodel.Segment {
	item, err := store.Get(datakinds.Segments, key)
	require.NoError(t, err)
	require.NotNil(t, item.Item)
	return item.Item.(*ldmodel.Segment)
}

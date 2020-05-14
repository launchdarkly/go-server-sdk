package ldfiledata

import (
	"io/ioutil"
	"os"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTempFile(t *testing.T, initialText string) string {
	f, err := ioutil.TempFile("", "file-dataSource-test")
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

func TestNewFileDataSourceYaml(t *testing.T) {
	filename := makeTempFile(t, `
---
flags:
  my-flag:
    "on": true
segments:
  my-segment:
    rules: []
`)
	defer os.Remove(filename)

	store := makeDataStore()

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())
	flagItem, err := store.Get(interfaces.DataKindFeatures(), "my-flag")
	require.NoError(t, err)
	require.NotNil(t, flagItem.Item)
	assert.True(t, flagItem.Item.(*ldmodel.FeatureFlag).On)

	segmentItem, err := store.Get(interfaces.DataKindSegments(), "my-segment")
	require.NoError(t, err)
	require.NotNil(t, segmentItem.Item)
	assert.Empty(t, segmentItem.Item.(*ldmodel.Segment).Rules)
}

func TestNewFileDataSourceJson(t *testing.T) {
	filename := makeTempFile(t, `{"flags": {"my-flag": {"on": true}}}`)
	defer os.Remove(filename)

	store := makeDataStore()

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())
	flagItem, err := store.Get(interfaces.DataKindFeatures(), "my-flag")
	require.NoError(t, err)
	require.NotNil(t, flagItem.Item)
	assert.True(t, flagItem.Item.(*ldmodel.FeatureFlag).On)
}

func TestNewFileDataSourceJsonWithTwoFiles(t *testing.T) {
	filename1 := makeTempFile(t, `{"flags": {"my-flag1": {"on": true}}}`)
	defer os.Remove(filename1)
	filename2 := makeTempFile(t, `{"flags": {"my-flag2": {"on": true}}}`)
	defer os.Remove(filename2)

	store := makeDataStore()

	factory := NewFileDataSourceFactory(FilePaths(filename1, filename2))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())

	flagItem1, err := store.Get(interfaces.DataKindFeatures(), "my-flag1")
	require.NoError(t, err)
	require.NotNil(t, flagItem1.Item)
	assert.True(t, flagItem1.Item.(*ldmodel.FeatureFlag).On)

	flagItem2, err := store.Get(interfaces.DataKindFeatures(), "my-flag2")
	require.NoError(t, err)
	require.NotNil(t, flagItem2.Item)
	assert.True(t, flagItem2.Item.(*ldmodel.FeatureFlag).On)
}

func TestNewFileDataSourceJsonWithTwoConflictingFiles(t *testing.T) {
	filename1 := makeTempFile(t, `{"flags": {"my-flag1": {"on": true}}}`)
	defer os.Remove(filename1)
	filename2 := makeTempFile(t, `{"flags": {"my-flag1": {"on": true}}}`)
	defer os.Remove(filename2)

	store := makeDataStore()

	factory := NewFileDataSourceFactory(FilePaths(filename1, filename2))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.False(t, dataSource.Initialized())
}

func TestNewFileDataSourceBadData(t *testing.T) {
	filename := makeTempFile(t, `bad data`)
	defer os.Remove(filename)

	store := makeDataStore()

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	assert.False(t, dataSource.Initialized())
}

func TestNewFileDataSourceMissingFile(t *testing.T) {
	filename := makeTempFile(t, "")
	os.Remove(filename)

	store := makeDataStore()

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	assert.False(t, dataSource.Initialized())
}

func TestNewFileDataSourceYamlValues(t *testing.T) {
	filename := makeTempFile(t, `
---
flagValues:
  my-flag: true
`)
	defer os.Remove(filename)

	store := makeDataStore()

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory.CreateDataSource(testContext(), store)
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())
	flagItem, err := store.Get(interfaces.DataKindFeatures(), "my-flag")
	require.NoError(t, err)
	require.NotNil(t, flagItem.Item)
	assert.Equal(t, []ldvalue.Value{ldvalue.Bool(true)}, flagItem.Item.(*ldmodel.FeatureFlag).Variations)
}

package ldfiledata

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
)

func makeTempFile(t *testing.T, initialText string) string {
	f, err := ioutil.TempFile("", "file-dataSource-test")
	require.NoError(t, err)
	f.WriteString(initialText)
	require.NoError(t, f.Close())
	return f.Name()
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

	store := ld.NewInMemoryFeatureStore(nil)

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory("", ld.Config{FeatureStore: store})
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())
	flag, err := store.Get(ld.Features, "my-flag")
	require.NoError(t, err)
	require.NotNil(t, flag)
	assert.True(t, flag.(*ld.FeatureFlag).On)

	segment, err := store.Get(ld.Segments, "my-segment")
	require.NoError(t, err)
	require.NotNil(t, segment)
	assert.Empty(t, segment.(*ld.Segment).Rules)
}

func TestNewFileDataSourceJson(t *testing.T) {
	filename := makeTempFile(t, `{"flags": {"my-flag": {"on": true}}}`)
	defer os.Remove(filename)

	store := ld.NewInMemoryFeatureStore(nil)

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory("", ld.Config{FeatureStore: store})
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())
	flag, err := store.Get(ld.Features, "my-flag")
	require.NoError(t, err)
	require.NotNil(t, flag)
	assert.True(t, flag.(*ld.FeatureFlag).On)
}

func TestNewFileDataSourceJsonWithTwoFiles(t *testing.T) {
	filename1 := makeTempFile(t, `{"flags": {"my-flag1": {"on": true}}}`)
	defer os.Remove(filename1)
	filename2 := makeTempFile(t, `{"flags": {"my-flag2": {"on": true}}}`)
	defer os.Remove(filename2)

	store := ld.NewInMemoryFeatureStore(nil)

	factory := NewFileDataSourceFactory(FilePaths(filename1, filename2))
	dataSource, err := factory("", ld.Config{FeatureStore: store})
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())

	flag1, err := store.Get(ld.Features, "my-flag1")
	require.NoError(t, err)
	require.NotNil(t, flag1)
	assert.True(t, flag1.(*ld.FeatureFlag).On)

	flag2, err := store.Get(ld.Features, "my-flag2")
	require.NoError(t, err)
	require.NotNil(t, flag2)
	assert.True(t, flag2.(*ld.FeatureFlag).On)
}

func TestNewFileDataSourceJsonWithTwoConflictingFiles(t *testing.T) {
	filename1 := makeTempFile(t, `{"flags": {"my-flag1": {"on": true}}}`)
	defer os.Remove(filename1)
	filename2 := makeTempFile(t, `{"flags": {"my-flag1": {"on": true}}}`)
	defer os.Remove(filename2)

	store := ld.NewInMemoryFeatureStore(nil)

	factory := NewFileDataSourceFactory(FilePaths(filename1, filename2))
	dataSource, err := factory("", ld.Config{FeatureStore: store})
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.False(t, dataSource.Initialized())
}

func TestNewFileDataSourceBadData(t *testing.T) {
	filename := makeTempFile(t, `bad data`)
	defer os.Remove(filename)

	store := ld.NewInMemoryFeatureStore(nil)

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory("", ld.Config{FeatureStore: store})
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	assert.False(t, dataSource.Initialized())
}

func TestNewFileDataSourceMissingFile(t *testing.T) {
	filename := makeTempFile(t, "")
	os.Remove(filename)

	store := ld.NewInMemoryFeatureStore(nil)

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory("", ld.Config{FeatureStore: store})
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

	store := ld.NewInMemoryFeatureStore(nil)

	factory := NewFileDataSourceFactory(FilePaths(filename))
	dataSource, err := factory("", ld.Config{FeatureStore: store})
	require.NoError(t, err)
	closeWhenReady := make(chan struct{})
	dataSource.Start(closeWhenReady)
	<-closeWhenReady
	require.True(t, dataSource.Initialized())
	flag, err := store.Get(ld.Features, "my-flag")
	require.NoError(t, err)
	require.NotNil(t, flag)
	require.NotNil(t, flag.(*ld.FeatureFlag).Fallthrough.Variation)
	require.True(t, flag.(*ld.FeatureFlag).On)
	assert.Equal(t, 0, *flag.(*ld.FeatureFlag).Fallthrough.Variation)
}

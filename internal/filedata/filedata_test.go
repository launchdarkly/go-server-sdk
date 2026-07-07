package filedata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "filedata-test")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestReadFileJSON(t *testing.T) {
	path := writeTempFile(t, `{"flagValues": {"my-flag": true}, "segments": {"my-segment": {"key": "my-segment", "version": 3}}}`)
	doc, err := ReadFile(path)
	require.NoError(t, err)
	require.NotNil(t, doc.FlagValues)
	assert.Equal(t, ldvalue.Bool(true), (*doc.FlagValues)["my-flag"])
	require.NotNil(t, doc.Segments)
	assert.Equal(t, 3, (*doc.Segments)["my-segment"].Version)
	assert.Nil(t, doc.Flags)
}

func TestReadFileYAML(t *testing.T) {
	path := writeTempFile(t, "flagValues:\n  my-flag: yes\n")
	doc, err := ReadFile(path)
	require.NoError(t, err)
	require.NotNil(t, doc.FlagValues)
	assert.Equal(t, ldvalue.Bool(true), (*doc.FlagValues)["my-flag"])
}

func TestReadFileErrors(t *testing.T) {
	_, err := ReadFile(filepath.Join(t.TempDir(), "nonexistent"))
	assert.ErrorContains(t, err, "unable to read file")

	path := writeTempFile(t, `{"flagValues"`)
	_, err = ReadFile(path)
	assert.ErrorContains(t, err, "error parsing file")

	path = writeTempFile(t, "\t: not yaml")
	_, err = ReadFile(path)
	assert.ErrorContains(t, err, "error parsing file")
}

func TestAbsFilePaths(t *testing.T) {
	abs, err := AbsFilePaths([]string{"relative/path", string(filepath.Separator) + "already-absolute"})
	require.NoError(t, err)
	require.Len(t, abs, 2)
	for _, p := range abs {
		assert.True(t, filepath.IsAbs(p), "expected absolute path, got %s", p)
	}
}

func TestMakeFlagWithValue(t *testing.T) {
	flag := MakeFlagWithValue("my-flag", "on")
	assert.Equal(t, "my-flag", flag.Key)
	require.Len(t, flag.Variations, 1)
	assert.Equal(t, ldvalue.String("on"), flag.Variations[0])
	require.NotNil(t, flag.OffVariation)
}

func docWithFlag(flag ldmodel.FeatureFlag) Document {
	flags := map[string]ldmodel.FeatureFlag{flag.Key: flag}
	return Document{Flags: &flags}
}

func docWithFlagValue(key string, value ldvalue.Value) Document {
	values := map[string]ldvalue.Value{key: value}
	return Document{FlagValues: &values}
}

func docWithSegment(segment ldmodel.Segment) Document {
	segments := map[string]ldmodel.Segment{segment.Key: segment}
	return Document{Segments: &segments}
}

func TestMergeCombinesDocuments(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(2).Build()
	segment1 := ldbuilders.NewSegmentBuilder("segment1").Version(4).Build()

	result, err := Merge(DuplicateKeysFail,
		docWithFlag(flag1), docWithFlagValue("flag2", ldvalue.Bool(true)), docWithSegment(segment1))
	require.NoError(t, err)

	require.Len(t, result.Flags, 2)
	assert.Equal(t, "flag1", result.Flags[0].Key)
	assert.Equal(t, 2, result.Flags[0].Item.Version)
	require.IsType(t, &ldmodel.FeatureFlag{}, result.Flags[0].Item.Item)
	assert.Equal(t, "flag2", result.Flags[1].Key)
	expanded := result.Flags[1].Item.Item.(*ldmodel.FeatureFlag)
	require.Len(t, expanded.Variations, 1)
	assert.Equal(t, ldvalue.Bool(true), expanded.Variations[0])

	require.Len(t, result.Segments, 1)
	assert.Equal(t, "segment1", result.Segments[0].Key)
	assert.Equal(t, 4, result.Segments[0].Item.Version)
}

func TestMergeDuplicateKeys(t *testing.T) {
	flagA := ldbuilders.NewFlagBuilder("flag1").Version(1).Build()
	flagB := ldbuilders.NewFlagBuilder("flag1").Version(2).Build()

	t.Run("fail", func(t *testing.T) {
		_, err := Merge(DuplicateKeysFail, docWithFlag(flagA), docWithFlag(flagB))
		assert.ErrorContains(t, err, "flag 'flag1' is specified by multiple files")
	})

	t.Run("unrecognized handling behaves as fail", func(t *testing.T) {
		_, err := Merge(DuplicateKeysHandling("bogus"), docWithFlag(flagA), docWithFlag(flagB))
		assert.Error(t, err)
	})

	t.Run("ignore all but first", func(t *testing.T) {
		result, err := Merge(DuplicateKeysIgnoreAllButFirst, docWithFlag(flagA), docWithFlag(flagB))
		require.NoError(t, err)
		require.Len(t, result.Flags, 1)
		assert.Equal(t, 1, result.Flags[0].Item.Version)
	})

	t.Run("full flag and flag value collide", func(t *testing.T) {
		_, err := Merge(DuplicateKeysFail, docWithFlag(flagA), docWithFlagValue("flag1", ldvalue.Bool(true)))
		assert.Error(t, err)
	})

	t.Run("duplicate segments", func(t *testing.T) {
		segment := ldbuilders.NewSegmentBuilder("segment1").Build()
		_, err := Merge(DuplicateKeysFail, docWithSegment(segment), docWithSegment(segment))
		assert.ErrorContains(t, err, "segment 'segment1' is specified by multiple files")
	})
}

func TestMergePreservesDocumentOrder(t *testing.T) {
	docs := make([]Document, 0, 5)
	expectedKeys := []string{"flag-a", "flag-b", "flag-c", "flag-d", "flag-e"}
	for _, key := range expectedKeys {
		docs = append(docs, docWithFlag(ldbuilders.NewFlagBuilder(key).Build()))
	}
	result, err := Merge(DuplicateKeysFail, docs...)
	require.NoError(t, err)
	keys := make([]string, 0, len(result.Flags))
	for _, item := range result.Flags {
		keys = append(keys, item.Key)
	}
	assert.Equal(t, expectedKeys, keys)
}

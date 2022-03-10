package datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v2/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/sharedtest"
)

func TestParsePutData(t *testing.T) {
	allDataJSON := `{
 "flags": {
  "flag1": {"key": "flag1", "version": 1},
  "flag2": {"key": "flag2", "version": 2}
 },
 "segments": {
  "segment1": {"key": "segment1","version": 3}
 }
}`
	expectedAllData := sharedtest.NewDataSetBuilder().
		Flags(ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
			ldbuilders.NewFlagBuilder("flag2").Version(2).Build()).
		Segments(ldbuilders.NewSegmentBuilder("segment1").Version(3).Build()).
		Build()

	t.Run("valid", func(t *testing.T) {
		input := []byte(`{"path": "/", "data": ` + allDataJSON + `}`)

		result, err := parsePutData(input)
		require.NoError(t, err)

		assert.Equal(t, "/", result.Path)
		assert.Equal(t, sharedtest.NormalizeDataSet(expectedAllData), sharedtest.NormalizeDataSet(result.Data))
	})

	t.Run("missing path", func(t *testing.T) {
		input := []byte(`{"data": ` + allDataJSON + `}`)
		result, err := parsePutData(input)
		require.NoError(t, err) // we don't consider this an error; some versions of Relay don't send a path
		assert.Equal(t, "", result.Path)
		assert.Equal(t, sharedtest.NormalizeDataSet(expectedAllData), sharedtest.NormalizeDataSet(result.Data))
	})

	t.Run("missing data", func(t *testing.T) {
		input := []byte(`{"path": "/"}`)
		_, err := parsePutData(input)
		require.Error(t, err)
	})
}

func TestParsePatchData(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagkey").Version(2).On(true).Build()
	segment := ldbuilders.NewSegmentBuilder("segmentkey").Version(3).Included("x").Build()
	flagJSON := `{"key": "flagkey", "version": 2, "on": true}`
	segmentJSON := `{"key": "segmentkey", "version": 3, "included": ["x"]}`

	t.Run("valid flag", func(t *testing.T) {
		input := []byte(`{"path": "/flags/flagkey", "data": ` + flagJSON + `}`)
		result, err := parsePatchData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.Features, result.Kind)
		assert.Equal(t, "flagkey", result.Key)
		assert.Equal(t, sharedtest.FlagDescriptor(flag), result.Data)
	})

	t.Run("valid segment", func(t *testing.T) {
		input := []byte(`{"path": "/segments/segmentkey", "data": ` + segmentJSON + `}`)
		result, err := parsePatchData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.Segments, result.Kind)
		assert.Equal(t, "segmentkey", result.Key)
		assert.Equal(t, sharedtest.SegmentDescriptor(segment), result.Data)
	})

	t.Run("valid but data property appears before path", func(t *testing.T) {
		input := []byte(`{"data": ` + flagJSON + `, "path": "/flags/flagkey"}`)
		result, err := parsePatchData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.Features, result.Kind)
		assert.Equal(t, "flagkey", result.Key)
		assert.Equal(t, sharedtest.FlagDescriptor(flag), result.Data)
	})

	t.Run("unrecognized path", func(t *testing.T) {
		input := []byte(`{"path": "/cats/lucy", "data": ` + flagJSON + `}`)
		result, err := parsePatchData(input)
		require.NoError(t, err)

		assert.Nil(t, result.Kind)
		assert.Equal(t, "", result.Key)
	})

	t.Run("missing path", func(t *testing.T) {
		input := []byte(`{"data": ` + flagJSON + `}`)
		_, err := parsePatchData(input)
		require.Error(t, err)
	})

	t.Run("missing data", func(t *testing.T) {
		input := []byte(`{"path": "/flags/flagkey"}`)
		_, err := parsePatchData(input)
		require.Error(t, err)
	})
}

func TestParseDeleteData(t *testing.T) {
	t.Run("valid flag", func(t *testing.T) {
		input := []byte(`{"path": "/flags/flagkey", "version": 3}`)
		result, err := parseDeleteData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.Features, result.Kind)
		assert.Equal(t, "flagkey", result.Key)
		assert.Equal(t, 3, result.Version)
	})

	t.Run("valid segment", func(t *testing.T) {
		input := []byte(`{"path": "/segments/segmentkey", "version": 4}`)
		result, err := parseDeleteData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.Segments, result.Kind)
		assert.Equal(t, "segmentkey", result.Key)
		assert.Equal(t, 4, result.Version)
	})

	t.Run("unrecognized path", func(t *testing.T) {
		input := []byte(`{"path": "/cats/macavity", "version": 9}`)
		result, err := parseDeleteData(input)
		require.NoError(t, err)

		assert.Nil(t, result.Kind)
		assert.Equal(t, "", result.Key)
	})

	t.Run("missing path", func(t *testing.T) {
		input := []byte(`{"version": 1}`)
		_, err := parseDeleteData(input)
		require.Error(t, err)
	})

	t.Run("missing version", func(t *testing.T) {
		input := []byte(`{"path": "/flags/flagkey"}`)
		_, err := parseDeleteData(input)
		require.Error(t, err)
	})
}

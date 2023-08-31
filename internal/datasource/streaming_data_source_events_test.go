package datasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
)

func TestParsePutData(t *testing.T) {
	allDataJSON := `{
 "flags": {
  "flag1": {"key": "flag1", "version": 1},
  "flag2": {"key": "flag2", "version": 2}
 },
 "segments": {
  "segment1": {"key": "segment1", "version": 3}
 },
 "configurationOverrides": {
  "override1": {"key": "override1", "version": 4}
 },
 "metrics": {
  "metric1": {"key": "metric1", "version": 5}
 }
}`
	expectedAllData := sharedtest.NewDataSetBuilder().
		Flags(ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
			ldbuilders.NewFlagBuilder("flag2").Version(2).Build()).
		Segments(ldbuilders.NewSegmentBuilder("segment1").Version(3).Build()).
		ConfigOverrides(ldbuilders.NewConfigOverrideBuilder("override1").Version(4).Build()).
		Metrics(ldbuilders.NewMetricBuilder("metric1").Version(5).Build()).
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
	configOverride := ldbuilders.NewConfigOverrideBuilder("indexSamplingRatio").Value(ldvalue.Int(5)).Version(4).Build()
	metric := ldbuilders.NewMetricBuilder("custom-metric").SamplingRatio(15).Version(5).Build()
	flagJSON := `{"key": "flagkey", "version": 2, "on": true}`
	segmentJSON := `{"key": "segmentkey", "version": 3, "included": ["x"]}`
	configOverrideJSON := `{"key": "indexSamplingRatio", "value": 5, "version": 4}`
	metricJSON := `{"key": "custom-metric", "samplingRatio": 15, "version": 5}`

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

	t.Run("valid config override", func(t *testing.T) {
		input := []byte(`{"path": "/configurationOverrides/indexSamplingRatio", "data": ` + configOverrideJSON + `}`)
		result, err := parsePatchData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.ConfigOverrides, result.Kind)
		assert.Equal(t, "indexSamplingRatio", result.Key)
		assert.Equal(t, sharedtest.ConfigOverrideDescriptor(configOverride), result.Data)
	})

	t.Run("valid metric", func(t *testing.T) {
		input := []byte(`{"path": "/metrics/custom-metric", "data": ` + metricJSON + `}`)
		result, err := parsePatchData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.Metrics, result.Kind)
		assert.Equal(t, "custom-metric", result.Key)
		assert.Equal(t, sharedtest.MetricDescriptor(metric), result.Data)
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

	t.Run("valid config overrides", func(t *testing.T) {
		input := []byte(`{"path": "/configurationOverrides/indexSamplingRatio", "version": 4}`)
		result, err := parseDeleteData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.ConfigOverrides, result.Kind)
		assert.Equal(t, "indexSamplingRatio", result.Key)
		assert.Equal(t, 4, result.Version)
	})

	t.Run("valid metric", func(t *testing.T) {
		input := []byte(`{"path": "/metrics/metrickey", "version": 4}`)
		result, err := parseDeleteData(input)
		require.NoError(t, err)

		assert.Equal(t, datakinds.Metrics, result.Kind)
		assert.Equal(t, "metrickey", result.Key)
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

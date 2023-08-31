package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"
)

func TestDataStoreEvalFeatures(t *testing.T) {
	store := fakeStoreForDataStoreProvider{}
	flag := ldbuilders.NewFlagBuilder("flagkey").Build()
	store.data = map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor{
		datakinds.Features: {
			flag.Key:      {Version: flag.Version, Item: &flag},
			"deleted-key": {Version: 9, Item: nil},
			"wrong-type":  {Version: 1, Item: "not a flag"},
		},
	}

	provider := NewDataStoreEvaluatorDataProviderImpl(store, ldlog.NewDisabledLoggers())

	assert.Equal(t, &flag, provider.GetFeatureFlag(flag.Key))
	assert.Nil(t, provider.GetFeatureFlag("unknown-key"))
	assert.Nil(t, provider.GetFeatureFlag("deleted-key"))
	assert.Nil(t, provider.GetFeatureFlag("wrong-type"))
}

func TestDataStoreEvalSegments(t *testing.T) {
	store := fakeStoreForDataStoreProvider{}
	segment := ldbuilders.NewSegmentBuilder("segmentkey").Build()
	store.data = map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor{
		datakinds.Segments: {
			segment.Key:   {Version: segment.Version, Item: &segment},
			"deleted-key": {Version: 9, Item: nil},
			"wrong-type":  {Version: 1, Item: "not a segment"},
		},
	}

	provider := NewDataStoreEvaluatorDataProviderImpl(store, ldlog.NewDisabledLoggers())

	assert.Equal(t, &segment, provider.GetSegment(segment.Key))
	assert.Nil(t, provider.GetSegment("unknown-key"))
	assert.Nil(t, provider.GetSegment("deleted-key"))
	assert.Nil(t, provider.GetSegment("wrong-type"))
}

func TestDataStoreEvalConfigOverrides(t *testing.T) {
	store := fakeStoreForDataStoreProvider{}
	override := ldbuilders.NewConfigOverrideBuilder("indexSamplingRatio").Build()
	store.data = map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor{
		datakinds.ConfigOverrides: {
			override.Key:  {Version: override.Version, Item: &override},
			"deleted-key": {Version: 9, Item: nil},
			"wrong-type":  {Version: 1, Item: "not an override"},
		},
	}

	provider := NewDataStoreEvaluatorDataProviderImpl(store, ldlog.NewDisabledLoggers())

	assert.Equal(t, &override, provider.GetConfigOverride(override.Key))
	assert.Nil(t, provider.GetConfigOverride("unknown-key"))
	assert.Nil(t, provider.GetConfigOverride("deleted-key"))
	assert.Nil(t, provider.GetConfigOverride("wrong-type"))
}

func TestDataStoreEvalMetrics(t *testing.T) {
	store := fakeStoreForDataStoreProvider{}
	metric := ldbuilders.NewMetricBuilder("custom-metric").Build()
	store.data = map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor{
		datakinds.Metrics: {
			metric.Key:    {Version: metric.Version, Item: &metric},
			"deleted-key": {Version: 9, Item: nil},
			"wrong-type":  {Version: 1, Item: "not an override"},
		},
	}

	provider := NewDataStoreEvaluatorDataProviderImpl(store, ldlog.NewDisabledLoggers())

	assert.Equal(t, &metric, provider.GetMetric(metric.Key))
	assert.Nil(t, provider.GetMetric("unknown-key"))
	assert.Nil(t, provider.GetMetric("deleted-key"))
	assert.Nil(t, provider.GetMetric("wrong-type"))
}

type fakeStoreForDataStoreProvider struct {
	data      map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor
	fakeError error
}

func (f fakeStoreForDataStoreProvider) Init(allData []ldstoretypes.Collection) error {
	return nil
}

func (f fakeStoreForDataStoreProvider) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	if f.fakeError != nil {
		return ldstoretypes.ItemDescriptor{}, f.fakeError
	}
	return f.data[kind][key], nil
}

func (f fakeStoreForDataStoreProvider) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	return nil, nil
}

func (f fakeStoreForDataStoreProvider) Upsert(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) (bool, error) {
	return false, nil
}

func (f fakeStoreForDataStoreProvider) IsInitialized() bool {
	return false
}

func (f fakeStoreForDataStoreProvider) IsStatusMonitoringEnabled() bool {
	return false
}

func (f fakeStoreForDataStoreProvider) Close() error {
	return nil
}

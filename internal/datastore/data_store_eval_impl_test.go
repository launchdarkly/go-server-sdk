package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
)

func TestDataStoreEvalFeatures(t *testing.T) {
	store := fakeStoreForDataStoreProvider{}
	flag := ldbuilders.NewFlagBuilder("flagkey").Build()
	store.data = map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor{
		datakinds.Features: map[string]ldstoretypes.ItemDescriptor{
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
		datakinds.Segments: map[string]ldstoretypes.ItemDescriptor{
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

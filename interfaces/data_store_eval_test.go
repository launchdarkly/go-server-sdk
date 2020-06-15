package interfaces

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
)

func TestDataStoreEvalFeatures(t *testing.T) {
	store := fakeStoreForDataStoreProvider{}
	flag := ldbuilders.NewFlagBuilder("flagkey").Build()
	store.data = map[StoreDataKind]map[string]StoreItemDescriptor{
		DataKindFeatures(): map[string]StoreItemDescriptor{
			flag.Key:      StoreItemDescriptor{Version: flag.Version, Item: &flag},
			"deleted-key": StoreItemDescriptor{Version: 9, Item: nil},
			"wrong-type":  StoreItemDescriptor{Version: 1, Item: "not a flag"},
		},
	}

	provider := NewDataStoreEvaluatorDataProvider(store, ldlog.NewDisabledLoggers())

	assert.Equal(t, &flag, provider.GetFeatureFlag(flag.Key))
	assert.Nil(t, provider.GetFeatureFlag("unknown-key"))
	assert.Nil(t, provider.GetFeatureFlag("deleted-key"))
	assert.Nil(t, provider.GetFeatureFlag("wrong-type"))
}

func TestDataStoreEvalSegments(t *testing.T) {
	store := fakeStoreForDataStoreProvider{}
	segment := ldbuilders.NewSegmentBuilder("segmentkey").Build()
	store.data = map[StoreDataKind]map[string]StoreItemDescriptor{
		DataKindSegments(): map[string]StoreItemDescriptor{
			segment.Key:   StoreItemDescriptor{Version: segment.Version, Item: &segment},
			"deleted-key": StoreItemDescriptor{Version: 9, Item: nil},
			"wrong-type":  StoreItemDescriptor{Version: 1, Item: "not a segment"},
		},
	}

	provider := NewDataStoreEvaluatorDataProvider(store, ldlog.NewDisabledLoggers())

	assert.Equal(t, &segment, provider.GetSegment(segment.Key))
	assert.Nil(t, provider.GetSegment("unknown-key"))
	assert.Nil(t, provider.GetSegment("deleted-key"))
	assert.Nil(t, provider.GetSegment("wrong-type"))
}

type fakeStoreForDataStoreProvider struct {
	data      map[StoreDataKind]map[string]StoreItemDescriptor
	fakeError error
}

func (f fakeStoreForDataStoreProvider) Init(allData []StoreCollection) error {
	return nil
}

func (f fakeStoreForDataStoreProvider) Get(kind StoreDataKind, key string) (StoreItemDescriptor, error) {
	if f.fakeError != nil {
		return StoreItemDescriptor{}, f.fakeError
	}
	return f.data[kind][key], nil
}

func (f fakeStoreForDataStoreProvider) GetAll(kind StoreDataKind) ([]StoreKeyedItemDescriptor, error) {
	return nil, nil
}

func (f fakeStoreForDataStoreProvider) Upsert(kind StoreDataKind, key string, item StoreItemDescriptor) (bool, error) {
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

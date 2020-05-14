package ldcomponents

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/ldservices"
	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

type dataSourceTestParams struct {
	store                   *dataStoreThatCapturesUpdates
	dataStoreUpdates        *internal.DataStoreUpdatesImpl
	dataStoreStatusProvider interfaces.DataStoreStatusProvider
}

func withDataSourceTestParams(action func(dataSourceTestParams)) {
	params := dataSourceTestParams{}
	params.store = newDataStoreThatCapturesUpdates()
	broadcaster := internal.NewDataStoreStatusBroadcaster()
	defer broadcaster.Close()
	params.dataStoreUpdates = internal.NewDataStoreUpdatesImpl(broadcaster)
	params.dataStoreStatusProvider = internal.NewDataStoreStatusProviderImpl(params.store, params.dataStoreUpdates)

	action(params)
}

func (p dataSourceTestParams) waitForInit(
	t *testing.T,
	data *ldservices.ServerSDKData,
) {
	select {
	case inited := <-p.store.receivedInits:
		assertReceivedInitDataEquals(t, data, inited)
		break
	case <-time.After(time.Second * 3):
		assert.Fail(t, "timed out before receiving expected init")
	}
}

func (p dataSourceTestParams) waitForUpdate(
	t *testing.T,
	kind interfaces.StoreDataKind,
	key string,
	version int,
) {
	select {
	case upserted := <-p.store.receivedUpserts:
		assert.Equal(t, key, upserted.key)
		assert.Equal(t, version, upserted.item.Version)
		assert.NotNil(t, upserted.item.Item)
		break
	case <-time.After(time.Second * 3):
		assert.Fail(t, "timed out before receiving expected update")
	}
}

func (p dataSourceTestParams) waitForDelete(
	t *testing.T,
	kind interfaces.StoreDataKind,
	key string,
	version int,
) {
	select {
	case upserted := <-p.store.receivedUpserts:
		assert.Equal(t, key, upserted.key)
		assert.Equal(t, version, upserted.item.Version)
		assert.Nil(t, upserted.item.Item)
		break
	case <-time.After(time.Second * 3):
		assert.Fail(t, "timed out before receiving expected deletion")
	}
}

func assertReceivedInitDataEquals(t *testing.T, expected *ldservices.ServerSDKData, received []interfaces.StoreCollection) {
	assert.Equal(t, 2, len(received))
	for _, coll := range received {
		var itemsMap map[string]interface{}
		if coll.Kind == interfaces.DataKindFeatures() {
			itemsMap = expected.FlagsMap
		} else if coll.Kind == interfaces.DataKindSegments() {
			itemsMap = expected.SegmentsMap
		} else {
			assert.Fail(t, "received unknown data kind: %s", coll.Kind)
		}
		assert.Equal(t, len(itemsMap), len(coll.Items))
		for _, item := range coll.Items {
			found, ok := itemsMap[item.Key]
			assert.True(t, ok, item.Key)
			bytes, _ := json.Marshal(found)
			var props map[string]interface{}
			assert.NoError(t, json.Unmarshal(bytes, &props))
			assert.Equal(t, props["version"].(float64), float64(item.Item.Version))
		}
	}
}

type upsertParams struct {
	kind interfaces.StoreDataKind
	key  string
	item interfaces.StoreItemDescriptor
}

type dataStoreThatCapturesUpdates struct {
	receivedInits   chan []interfaces.StoreCollection
	receivedUpserts chan upsertParams
	fakeError       error
}

func newDataStoreThatCapturesUpdates() *dataStoreThatCapturesUpdates {
	return &dataStoreThatCapturesUpdates{
		receivedInits:   make(chan []interfaces.StoreCollection, 10),
		receivedUpserts: make(chan upsertParams, 10),
	}
}

func (d *dataStoreThatCapturesUpdates) Init(allData []interfaces.StoreCollection) error {
	d.receivedInits <- allData
	if d.fakeError != nil {
		return d.fakeError
	}
	return nil
}

func (d *dataStoreThatCapturesUpdates) Get(kind interfaces.StoreDataKind, key string) (interfaces.StoreItemDescriptor, error) {
	return interfaces.StoreItemDescriptor{Version: -1, Item: nil}, nil
}

func (d *dataStoreThatCapturesUpdates) GetAll(kind interfaces.StoreDataKind) ([]interfaces.StoreKeyedItemDescriptor, error) {
	return nil, nil
}

func (d *dataStoreThatCapturesUpdates) Upsert(kind interfaces.StoreDataKind, key string, newItem interfaces.StoreItemDescriptor) error {
	d.receivedUpserts <- upsertParams{kind, key, newItem}
	return d.fakeError
}

func (d *dataStoreThatCapturesUpdates) IsInitialized() bool {
	return true
}

func (d *dataStoreThatCapturesUpdates) IsStatusMonitoringEnabled() bool {
	return true
}

func (d *dataStoreThatCapturesUpdates) Close() error {
	return nil
}

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

func waitForInit(
	t *testing.T,
	store *testDataStoreWithStatus,
	data *ldservices.ServerSDKData,
) {
	select {
	case receivedInitialData := <-store.inits:
		assertReceivedInitDataEquals(t, data, receivedInitialData)
		break
	case <-time.After(time.Second * 3):
		assert.Fail(t, "timed out before receiving expected init")
	}
}

type testDataStoreWithStatus struct {
	inits     chan []interfaces.StoreCollection
	statusSub *testStatusSubscription
}

func (t *testDataStoreWithStatus) Get(kind interfaces.StoreDataKind, key string) (interfaces.StoreItemDescriptor, error) {
	return interfaces.StoreItemDescriptor{}.NotFound(), nil
}

func (t *testDataStoreWithStatus) GetAll(kind interfaces.StoreDataKind) ([]interfaces.StoreKeyedItemDescriptor, error) {
	return nil, nil
}

func (t *testDataStoreWithStatus) Init(data []interfaces.StoreCollection) error {
	t.inits <- data
	return nil
}

func (t *testDataStoreWithStatus) Upsert(kind interfaces.StoreDataKind, key string, item interfaces.StoreItemDescriptor) error {
	return nil
}

func (t *testDataStoreWithStatus) IsInitialized() bool {
	return true
}

func (t *testDataStoreWithStatus) IsStatusMonitoringEnabled() bool {
	return true
}

func (t *testDataStoreWithStatus) GetStoreStatus() internal.DataStoreStatus {
	return internal.DataStoreStatus{Available: true}
}

func (t *testDataStoreWithStatus) StatusSubscribe() internal.DataStoreStatusSubscription {
	t.statusSub = &testStatusSubscription{
		ch: make(chan internal.DataStoreStatus),
	}
	return t.statusSub
}

func (t *testDataStoreWithStatus) Close() error {
	return nil
}

func (t *testDataStoreWithStatus) publishStatus(status internal.DataStoreStatus) {
	if t.statusSub != nil {
		t.statusSub.ch <- status
	}
}

type testStatusSubscription struct {
	ch chan internal.DataStoreStatus
}

func (s *testStatusSubscription) Channel() <-chan internal.DataStoreStatus {
	return s.ch
}

func (s *testStatusSubscription) Close() {
	close(s.ch)
}

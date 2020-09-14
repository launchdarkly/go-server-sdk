package sharedtest

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-test-helpers/v2/ldservices"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
)

// CapturingDataStore is a DataStore implementation that records update operations for testing.
type CapturingDataStore struct {
	realStore               interfaces.DataStore
	statusMonitoringEnabled bool
	fakeError               error
	inits                   chan []ldstoretypes.Collection
	upserts                 chan UpsertParams
	lock                    sync.Mutex
}

// UpsertParams holds the parameters of an Upsert operation captured by CapturingDataStore.
type UpsertParams struct {
	Kind ldstoretypes.DataKind
	Key  string
	Item ldstoretypes.ItemDescriptor
}

// NewCapturingDataStore creates an instance of CapturingDataStore.
func NewCapturingDataStore(realStore interfaces.DataStore) *CapturingDataStore {
	return &CapturingDataStore{
		realStore:               realStore,
		inits:                   make(chan []ldstoretypes.Collection, 10),
		upserts:                 make(chan UpsertParams, 10),
		statusMonitoringEnabled: true,
	}
}

// Init is a standard DataStore method.
func (d *CapturingDataStore) Init(allData []ldstoretypes.Collection) error {
	for _, coll := range allData {
		AssertNotNil(coll.Kind)
	}
	d.inits <- allData
	_ = d.realStore.Init(allData)
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.fakeError
}

// Get is a standard DataStore method.
func (d *CapturingDataStore) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	AssertNotNil(kind)
	if d.fakeError != nil {
		return ldstoretypes.ItemDescriptor{}.NotFound(), d.fakeError
	}
	return d.realStore.Get(kind, key)
}

// GetAll is a standard DataStore method.
func (d *CapturingDataStore) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	AssertNotNil(kind)
	if d.fakeError != nil {
		return nil, d.fakeError
	}
	return d.realStore.GetAll(kind)
}

// Upsert in this test type does nothing but capture its parameters.
func (d *CapturingDataStore) Upsert(
	kind ldstoretypes.DataKind,
	key string,
	newItem ldstoretypes.ItemDescriptor,
) (bool, error) {
	AssertNotNil(kind)
	d.upserts <- UpsertParams{kind, key, newItem}
	updated, _ := d.realStore.Upsert(kind, key, newItem)
	d.lock.Lock()
	defer d.lock.Unlock()
	return updated, d.fakeError
}

// IsInitialized in this test type always returns true.
func (d *CapturingDataStore) IsInitialized() bool {
	return true
}

// IsStatusMonitoringEnabled in this test type returns true by default, but can be changed
// with SetStatusMonitoringEnabled.
func (d *CapturingDataStore) IsStatusMonitoringEnabled() bool {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.statusMonitoringEnabled
}

// Close in this test type is a no-op.
func (d *CapturingDataStore) Close() error {
	return nil
}

// SetStatusMonitoringEnabled changes the value returned by IsStatusMonitoringEnabled.
func (d *CapturingDataStore) SetStatusMonitoringEnabled(statusMonitoringEnabled bool) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.statusMonitoringEnabled = statusMonitoringEnabled
}

// SetFakeError causes subsequent Init or Upsert calls to return an error.
func (d *CapturingDataStore) SetFakeError(fakeError error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.fakeError = fakeError
}

// WaitForNextInit waits for an Init call.
func (d *CapturingDataStore) WaitForNextInit(
	t *testing.T,
	timeout time.Duration,
) []ldstoretypes.Collection {
	select {
	case inited := <-d.inits:
		return inited
	case <-time.After(timeout):
		require.Fail(t, "timed out before receiving expected init")
	}
	return nil
}

// WaitForInit waits for an Init call and verifies that it matches the expected data.
func (d *CapturingDataStore) WaitForInit(
	t *testing.T,
	data *ldservices.ServerSDKData,
	timeout time.Duration,
) {
	select {
	case inited := <-d.inits:
		assertReceivedInitDataEquals(t, data, inited)
		break
	case <-time.After(timeout):
		require.Fail(t, "timed out before receiving expected init")
	}
}

// WaitForNextUpsert waits for an Upsert call.
func (d *CapturingDataStore) WaitForNextUpsert(
	t *testing.T,
	timeout time.Duration,
) UpsertParams {
	select {
	case upserted := <-d.upserts:
		return upserted
	case <-time.After(timeout):
		require.Fail(t, "timed out before receiving expected update")
		return UpsertParams{}
	}
}

// WaitForUpsert waits for an Upsert call and verifies that it matches the expected data.
func (d *CapturingDataStore) WaitForUpsert(
	t *testing.T,
	kind ldstoretypes.DataKind,
	key string,
	version int,
	timeout time.Duration,
) UpsertParams {
	upserted := d.WaitForNextUpsert(t, timeout)
	assert.Equal(t, kind, upserted.Kind)
	assert.Equal(t, key, upserted.Key)
	assert.Equal(t, version, upserted.Item.Version)
	assert.NotNil(t, upserted.Item.Item)
	return upserted
}

// WaitForDelete waits for an Upsert call that is expected to delete a data item.
func (d *CapturingDataStore) WaitForDelete(
	t *testing.T,
	kind ldstoretypes.DataKind,
	key string,
	version int,
	timeout time.Duration,
) {
	upserted := d.WaitForNextUpsert(t, timeout)
	assert.Equal(t, kind, upserted.Kind)
	assert.Equal(t, key, upserted.Key)
	assert.Equal(t, version, upserted.Item.Version)
	assert.Nil(t, upserted.Item.Item)
}

func assertReceivedInitDataEquals(
	t *testing.T,
	expected *ldservices.ServerSDKData,
	received []ldstoretypes.Collection,
) {
	assert.Equal(t, 2, len(received))
	for _, coll := range received {
		var itemsMap map[string]interface{}
		switch coll.Kind {
		case datakinds.Features:
			itemsMap = expected.FlagsMap
		case datakinds.Segments:
			itemsMap = expected.SegmentsMap
		default:
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

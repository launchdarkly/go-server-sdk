package mocks

import (
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
)

// MockDataDestination is a mock implementation of a data destination used tests involving FDv2 data sources.
type MockDataDestination struct {
	DataStore               *CapturingDataStore
	Statuses                chan interfaces.DataSourceStatus
	dataStoreStatusProvider *mockDataStoreStatusProvider
	lastStatus              interfaces.DataSourceStatus
	lock                    sync.Mutex
}

// NewMockDataDestination creates an instance of MockDataDestination.
//
// The DataStoreStatusProvider can be nil if we are not doing a test that requires manipulation of that
// component.
func NewMockDataDestination(realStore subsystems.DataStore) *MockDataDestination {
	dataStore := NewCapturingDataStore(realStore)
	dataStoreStatusProvider := &mockDataStoreStatusProvider{
		dataStore: dataStore,
		status:    interfaces.DataStoreStatus{Available: true},
		statusCh:  make(chan interfaces.DataStoreStatus, 10),
	}
	return &MockDataDestination{
		DataStore:               dataStore,
		Statuses:                make(chan interfaces.DataSourceStatus, 10),
		dataStoreStatusProvider: dataStoreStatusProvider,
	}
}

// Init in this test implementation, delegates to d.DataStore.CapturedUpdates.
func (d *MockDataDestination) Init(allData []ldstoretypes.Collection, _ *int) bool {
	// For now, the payloadVersion is ignored. When the data sources start making use of it, it should be
	// stored so that assertions can be made.
	for _, coll := range allData {
		AssertNotNil(coll.Kind)
	}
	err := d.DataStore.Init(allData)
	return err == nil
}

// Upsert in this test implementation, delegates to d.DataStore.CapturedUpdates.
func (d *MockDataDestination) Upsert(
	kind ldstoretypes.DataKind,
	key string,
	newItem ldstoretypes.ItemDescriptor,
) bool {
	AssertNotNil(kind)
	_, err := d.DataStore.Upsert(kind, key, newItem)
	return err == nil
}

// UpdateStatus in this test implementation, pushes a value onto the Statuses channel.
func (d *MockDataDestination) UpdateStatus(
	newState interfaces.DataSourceState,
	newError interfaces.DataSourceErrorInfo,
) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if newState != d.lastStatus.State || newError.Kind != "" {
		d.lastStatus = interfaces.DataSourceStatus{State: newState, LastError: newError}
		d.Statuses <- d.lastStatus
	}
}

// GetDataStoreStatusProvider returns a stub implementation that does not have full functionality
// but enough to test a data source with.
func (d *MockDataDestination) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return d.dataStoreStatusProvider
}

// UpdateStoreStatus simulates a change in the data store status.
func (d *MockDataDestination) UpdateStoreStatus(newStatus interfaces.DataStoreStatus) {
	d.dataStoreStatusProvider.statusCh <- newStatus
}

// RequireStatusOf blocks until a new data source status is available, and verifies its state.
func (d *MockDataDestination) RequireStatusOf(
	t *testing.T,
	newState interfaces.DataSourceState,
) interfaces.DataSourceStatus {
	status := d.RequireStatus(t)
	assert.Equal(t, string(newState), string(status.State))
	// string conversion is due to a bug in assert with type aliases
	return status
}

// RequireStatus blocks until a new data source status is available.
func (d *MockDataDestination) RequireStatus(t *testing.T) interfaces.DataSourceStatus {
	return th.RequireValue(t, d.Statuses, time.Second, "timed out waiting for new data source status")
}

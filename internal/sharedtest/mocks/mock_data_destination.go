package mocks

import (
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
)

// MockDataDestination is a mock implementation of a data destination used by tests involving FDv2 data sources.
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

// SetBasis in this test implementation, delegates to d.DataStore.CapturedUpdates.
func (d *MockDataDestination) SetBasis(events []fdv2proto.Event, selector fdv2proto.Selector, persist bool) error {
	// For now, the selector is ignored. When the data sources start making use of it, it should be
	// stored so that assertions can be made.

	collections := fdv2proto.ToStorableItems(events)

	for _, coll := range collections {
		AssertNotNil(coll.Kind)
	}
	return d.DataStore.Init(collections)
}

func (d *MockDataDestination) ApplyDelta(events []fdv2proto.Event, selector fdv2proto.Selector, persist bool) error {
	// For now, the selector is ignored. When the data sources start making use of it, it should be
	// stored so that assertions can be made.

	collections := fdv2proto.ToStorableItems(events)

	for _, coll := range collections {
		AssertNotNil(coll.Kind)
	}

	for _, coll := range collections {
		for _, item := range coll.Items {
			if _, err := d.DataStore.Upsert(coll.Kind, item.Key, item.Item); err != nil {
				return err
			}
		}
	}

	return nil
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

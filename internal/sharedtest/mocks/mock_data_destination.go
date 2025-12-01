package mocks

import (
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/toposort"

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
	lastKnownSelector       subsystems.Selector
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
		lastKnownSelector:       subsystems.NoSelector(),
	}
}

// Selector returns the last known selector value.
func (d *MockDataDestination) Selector() subsystems.Selector {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.lastKnownSelector
}

func (d *MockDataDestination) Apply(changeSet subsystems.ChangeSet, persist bool) {
	switch changeSet.IntentCode() {
	case subsystems.IntentTransferFull:
		d.setBasis(changeSet, persist)
	case subsystems.IntentTransferChanges:
		d.applyDelta(changeSet, persist)
	}
}

// setBasis in this test implementation, delegates to d.DataStore.CapturedUpdates.
func (d *MockDataDestination) setBasis(changeSet subsystems.ChangeSet, _ bool) {
	collections, err := changeSet.Collections()
	if err != nil {
		panic("MockDataDestination.setBasis received malformed data: " + err.Error())
	}

	for _, coll := range collections {
		AssertNotNil(coll.Kind)
	}

	if err := d.DataStore.Init(toposort.Sort(collections)); err == nil {
		d.lock.Lock()
		d.lastKnownSelector = changeSet.Selector()
		d.lock.Unlock()
	}
}

// applyDelta in this test implementation, delegates to d.DataStore.CapturedUpdates.
func (d *MockDataDestination) applyDelta(changeSet subsystems.ChangeSet, _ bool) {
	collections, err := changeSet.Collections()
	if err != nil {
		panic("MockDataDestination.applyDelta received malformed data: " + err.Error())
	}

	for _, coll := range collections {
		AssertNotNil(coll.Kind)
	}

	for _, coll := range toposort.Sort(collections) {
		for _, item := range coll.Items {
			if _, err := d.DataStore.Upsert(coll.Kind, item.Key, item.Item); err != nil {
				return
			}
		}
	}

	d.lock.Lock()
	d.lastKnownSelector = changeSet.Selector()
	d.lock.Unlock()
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

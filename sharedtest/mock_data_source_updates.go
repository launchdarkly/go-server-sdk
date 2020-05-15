package sharedtest

import (
	"sync"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// MockDataSourceUpdates is a mock implementation of DataSourceUpdates for testing data sources.
type MockDataSourceUpdates struct {
	DataStore               *CapturingDataStore
	Statuses                chan interfaces.DataSourceStatus
	dataStoreStatusProvider *mockDataStoreStatusProvider
}

// NewMockDataSourceUpdates creates an instance of MockDataSourceUpdates.
//
// The DataStoreStatusProvider can be nil if we are not doing a test that requires manipulation of that
// component.
func NewMockDataSourceUpdates(realStore interfaces.DataStore) *MockDataSourceUpdates {
	dataStore := NewCapturingDataStore(realStore)
	dataStoreStatusProvider := &mockDataStoreStatusProvider{
		dataStore: dataStore,
		status:    interfaces.DataStoreStatus{Available: true},
		statusCh:  make(chan interfaces.DataStoreStatus, 10),
	}
	return &MockDataSourceUpdates{
		DataStore:               dataStore,
		Statuses:                make(chan interfaces.DataSourceStatus, 10),
		dataStoreStatusProvider: dataStoreStatusProvider,
	}
}

// Init, in this test implementation, delegates to d.DataStore.CapturedUpdates.
func (d *MockDataSourceUpdates) Init(allData []interfaces.StoreCollection) bool {
	err := d.DataStore.Init(allData)
	return err == nil
}

// Upsert, in this test implementation, delegates to d.DataStore.CapturedUpdates.
func (d *MockDataSourceUpdates) Upsert(kind interfaces.StoreDataKind, key string, newItem interfaces.StoreItemDescriptor) bool {
	err := d.DataStore.Upsert(kind, key, newItem)
	return err == nil
}

// UpdateStatus, in this test implementation, pushes a value onto the Statuses channel.
func (d *MockDataSourceUpdates) UpdateStatus(newState interfaces.DataSourceState, newError interfaces.DataSourceErrorInfo) {
	d.Statuses <- interfaces.DataSourceStatus{State: newState, LastError: newError}
}

// GetDataStoreStatusProvider returns a stub implementation that does not have full functionality
// but enough to test a data source with.
func (d *MockDataSourceUpdates) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return d.dataStoreStatusProvider
}

// UpdateStoreStatus simulates a change in the data store status.
func (d *MockDataSourceUpdates) UpdateStoreStatus(newStatus interfaces.DataStoreStatus) {
	d.dataStoreStatusProvider.statusCh <- newStatus
}

type mockDataStoreStatusProvider struct {
	dataStore interfaces.DataStore
	status    interfaces.DataStoreStatus
	statusCh  chan interfaces.DataStoreStatus
	lock      sync.Mutex
}

func (m *mockDataStoreStatusProvider) GetStatus() interfaces.DataStoreStatus {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.status
}

func (m *mockDataStoreStatusProvider) IsStatusMonitoringEnabled() bool {
	return m.dataStore.IsStatusMonitoringEnabled()
}

func (m *mockDataStoreStatusProvider) AddStatusListener() <-chan interfaces.DataStoreStatus {
	return m.statusCh
}

func (m *mockDataStoreStatusProvider) RemoveStatusListener(ch <-chan interfaces.DataStoreStatus) {
}

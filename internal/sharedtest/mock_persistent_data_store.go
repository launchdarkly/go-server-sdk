package sharedtest

import (
	"sync"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
)

// MockDatabaseInstance can be used with MockPersistentDataStore to simulate multiple data store
// instances sharing the same underlying data space.
type MockDatabaseInstance struct {
	dataByPrefix   map[string]map[ldstoretypes.DataKind]map[string]ldstoretypes.SerializedItemDescriptor
	initedByPrefix map[string]*bool
}

// NewMockDatabaseInstance creates an instance of MockDatabaseInstance.
func NewMockDatabaseInstance() *MockDatabaseInstance {
	return &MockDatabaseInstance{
		dataByPrefix:   make(map[string]map[ldstoretypes.DataKind]map[string]ldstoretypes.SerializedItemDescriptor),
		initedByPrefix: make(map[string]*bool),
	}
}

// Clear removes all shared data.
func (db *MockDatabaseInstance) Clear(prefix string) {
	for _, m := range db.dataByPrefix[prefix] {
		for k := range m {
			delete(m, k)
		}
	}
	if v, ok := db.initedByPrefix[prefix]; ok {
		*v = false
	}
}

// MockPersistentDataStore is a test implementation of PersistentDataStore.
type MockPersistentDataStore struct {
	data                map[ldstoretypes.DataKind]map[string]ldstoretypes.SerializedItemDescriptor
	persistOnlyAsString bool
	fakeError           error
	available           bool
	inited              *bool
	InitQueriedCount    int
	queryDelay          time.Duration
	queryStartedCh      chan struct{}
	testTxHook          func()
	closed              bool
	lock                sync.Mutex
}

func newData() map[ldstoretypes.DataKind]map[string]ldstoretypes.SerializedItemDescriptor {
	return map[ldstoretypes.DataKind]map[string]ldstoretypes.SerializedItemDescriptor{
		MockData:      {},
		MockOtherData: {},
	}
}

// NewMockPersistentDataStore creates a test implementation of a persistent data store.
func NewMockPersistentDataStore() *MockPersistentDataStore {
	f := false
	m := &MockPersistentDataStore{data: newData(), inited: &f, available: true}
	return m
}

// NewMockPersistentDataStoreWithPrefix creates a test implementation of a persistent data store that uses
// a MockDatabaseInstance to simulate a shared database.
func NewMockPersistentDataStoreWithPrefix(
	db *MockDatabaseInstance,
	prefix string,
) *MockPersistentDataStore {
	m := &MockPersistentDataStore{available: true}
	if _, ok := db.dataByPrefix[prefix]; !ok {
		db.dataByPrefix[prefix] = newData()
		f := false
		db.initedByPrefix[prefix] = &f
	}
	m.data = db.dataByPrefix[prefix]
	m.inited = db.initedByPrefix[prefix]
	return m
}

// EnableInstrumentedQueries puts the test store into a mode where all get operations begin by posting
// a signal to a channel and then waiting for some amount of time, to test coalescing of requests.
func (m *MockPersistentDataStore) EnableInstrumentedQueries(queryDelay time.Duration) <-chan struct{} {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.queryDelay = queryDelay
	m.queryStartedCh = make(chan struct{}, 10)
	return m.queryStartedCh
}

// ForceGet retrieves a serialized item directly from the test data with no other processing.
func (m *MockPersistentDataStore) ForceGet(
	kind ldstoretypes.DataKind,
	key string,
) ldstoretypes.SerializedItemDescriptor {
	m.lock.Lock()
	defer m.lock.Unlock()
	if ret, ok := m.data[kind][key]; ok {
		return ret
	}
	return ldstoretypes.SerializedItemDescriptor{}.NotFound()
}

// ForceSet directly modifies an item in the test data.
func (m *MockPersistentDataStore) ForceSet(
	kind ldstoretypes.DataKind,
	key string,
	item ldstoretypes.SerializedItemDescriptor,
) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.data[kind][key] = item
}

// ForceRemove deletes an item from the test data.
func (m *MockPersistentDataStore) ForceRemove(kind ldstoretypes.DataKind, key string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.data[kind], key)
}

// ForceSetInited changes the value that will be returned by IsInitialized().
func (m *MockPersistentDataStore) ForceSetInited(inited bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	*m.inited = inited
}

// SetAvailable changes the value that will be returned by IsStoreAvailable().
func (m *MockPersistentDataStore) SetAvailable(available bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.available = available
}

// SetFakeError causes subsequent store operations to return an error.
func (m *MockPersistentDataStore) SetFakeError(fakeError error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.fakeError = fakeError
}

// SetPersistOnlyAsString sets whether the mock data store should behave like our Redis implementation,
// where the item version is *not* persisted separately from the serialized item string (so the latter must
// be parsed to get the version). If this is false (the default), it behaves instead like our DynamoDB
// implementation, where the version metadata exists separately from the serialized string.
func (m *MockPersistentDataStore) SetPersistOnlyAsString(value bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.persistOnlyAsString = value
}

// SetTestTxHook sets a callback function that will be called during updates, to support the concurrent
// modification tests in PersistentDataStoreTestSuite.
func (m *MockPersistentDataStore) SetTestTxHook(hook func()) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.testTxHook = hook
}

func (m *MockPersistentDataStore) startQuery() {
	if m.queryStartedCh != nil {
		m.queryStartedCh <- struct{}{}
	}
	if m.queryDelay > 0 {
		<-time.After(m.queryDelay)
	}
}

// Init is a standard PersistentDataStore method.
func (m *MockPersistentDataStore) Init(allData []ldstoretypes.SerializedCollection) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.fakeError != nil {
		return m.fakeError
	}
	for _, mm := range m.data {
		for k := range mm {
			delete(mm, k)
		}
	}
	for _, coll := range allData {
		AssertNotNil(coll.Kind)
		itemsMap := make(map[string]ldstoretypes.SerializedItemDescriptor)
		for _, item := range coll.Items {
			itemsMap[item.Key] = m.storableItem(item.Item)
		}
		m.data[coll.Kind] = itemsMap
	}
	*m.inited = true
	return nil
}

// Get is a standard PersistentDataStore method.
func (m *MockPersistentDataStore) Get(
	kind ldstoretypes.DataKind,
	key string,
) (ldstoretypes.SerializedItemDescriptor, error) {
	AssertNotNil(kind)
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.fakeError != nil {
		return ldstoretypes.SerializedItemDescriptor{}.NotFound(), m.fakeError
	}
	m.startQuery()
	if item, ok := m.data[kind][key]; ok {
		return m.retrievedItem(item), nil
	}
	return ldstoretypes.SerializedItemDescriptor{}.NotFound(), nil
}

// GetAll is a standard PersistentDataStore method.
func (m *MockPersistentDataStore) GetAll(
	kind ldstoretypes.DataKind,
) ([]ldstoretypes.KeyedSerializedItemDescriptor, error) {
	AssertNotNil(kind)
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.fakeError != nil {
		return nil, m.fakeError
	}
	m.startQuery()
	ret := []ldstoretypes.KeyedSerializedItemDescriptor{}
	for k, v := range m.data[kind] {
		ret = append(ret, ldstoretypes.KeyedSerializedItemDescriptor{Key: k, Item: m.retrievedItem(v)})
	}
	return ret, nil
}

// Upsert is a standard PersistentDataStore method.
func (m *MockPersistentDataStore) Upsert(
	kind ldstoretypes.DataKind,
	key string,
	newItem ldstoretypes.SerializedItemDescriptor,
) (bool, error) {
	AssertNotNil(kind)
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.fakeError != nil {
		return false, m.fakeError
	}
	if m.testTxHook != nil {
		m.testTxHook()
	}
	if oldItem, ok := m.data[kind][key]; ok {
		oldVersion := oldItem.Version
		if m.persistOnlyAsString {
			// If persistOnlyAsString is true, simulate the kind of implementation where we can't see the
			// version as a separate attribute in the database and must deserialize the item to get it.
			oldDeserializedItem, _ := kind.Deserialize(oldItem.SerializedItem)
			oldVersion = oldDeserializedItem.Version
		}
		if oldVersion >= newItem.Version {
			return false, nil
		}
	}
	m.data[kind][key] = m.storableItem(newItem)
	return true, nil
}

// IsInitialized is a standard PersistentDataStore method.
func (m *MockPersistentDataStore) IsInitialized() bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.InitQueriedCount++
	return *m.inited
}

// IsStoreAvailable is a standard PersistentDataStore method.
func (m *MockPersistentDataStore) IsStoreAvailable() bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.available
}

// Close is a standard PersistentDataStore method.
func (m *MockPersistentDataStore) Close() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.closed = true
	return nil
}

func (m *MockPersistentDataStore) retrievedItem(
	item ldstoretypes.SerializedItemDescriptor,
) ldstoretypes.SerializedItemDescriptor {
	if m.persistOnlyAsString {
		// This simulates the kind of store implementation that can't track metadata separately
		return ldstoretypes.SerializedItemDescriptor{Version: 0, SerializedItem: item.SerializedItem}
	}
	return item
}

func (m *MockPersistentDataStore) storableItem(
	item ldstoretypes.SerializedItemDescriptor,
) ldstoretypes.SerializedItemDescriptor {
	if item.Deleted && !m.persistOnlyAsString {
		// This simulates the kind of store implementation that *can* track metadata separately, so we don't
		// have to persist the placeholder string for deleted items
		return ldstoretypes.SerializedItemDescriptor{Version: item.Version, Deleted: true}
	}
	return item
}

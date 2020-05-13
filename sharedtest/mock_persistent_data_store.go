package sharedtest

import (
	"time"

	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type MockDatabaseInstance struct {
	dataByPrefix   map[string]map[intf.VersionedDataKind]map[string]intf.VersionedData
	initedByPrefix map[string]*bool
}

func NewMockDatabaseInstance() *MockDatabaseInstance {
	return &MockDatabaseInstance{
		dataByPrefix:   make(map[string]map[intf.VersionedDataKind]map[string]intf.VersionedData),
		initedByPrefix: make(map[string]*bool),
	}
}

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

type MockPersistentDataStore struct {
	Data             map[intf.VersionedDataKind]map[string]intf.VersionedData
	FakeError        error
	inited           *bool
	InitQueriedCount int
	queryCount       int
	queryDelay       time.Duration
	queryStartedCh   chan struct{}
	testTxHook       func()
	closed           bool
}

func newData() map[intf.VersionedDataKind]map[string]intf.VersionedData {
	return map[intf.VersionedDataKind]map[string]intf.VersionedData{
		MockData:      {},
		MockOtherData: {},
	}
}

func NewMockPersistentDataStore() *MockPersistentDataStore {
	f := false
	m := &MockPersistentDataStore{Data: newData(), inited: &f}
	return m
}

func NewMockPersistentDataStoreWithPrefix(db *MockDatabaseInstance, prefix string) *MockPersistentDataStore {
	m := &MockPersistentDataStore{}
	if _, ok := db.dataByPrefix[prefix]; !ok {
		db.dataByPrefix[prefix] = newData()
		f := false
		db.initedByPrefix[prefix] = &f
	}
	m.Data = db.dataByPrefix[prefix]
	m.inited = db.initedByPrefix[prefix]
	return m
}

func (m *MockPersistentDataStore) EnableInstrumentedQueries(queryDelay time.Duration) <-chan struct{} {
	m.queryDelay = queryDelay
	m.queryStartedCh = make(chan struct{}, 10)
	return m.queryStartedCh
}

func (m *MockPersistentDataStore) ForceGet(kind intf.VersionedDataKind, key string) intf.VersionedData {
	if ret, ok := m.Data[kind][key]; ok {
		return ret
	}
	return nil
}

func (m *MockPersistentDataStore) ForceSet(kind intf.VersionedDataKind, key string, item intf.VersionedData) {
	m.Data[kind][key] = item
}

func (m *MockPersistentDataStore) ForceRemove(kind intf.VersionedDataKind, key string) {
	delete(m.Data[kind], key)
}

func (m *MockPersistentDataStore) ForceSetInited(inited bool) {
	*m.inited = inited
}

func (m *MockPersistentDataStore) startQuery() {
	if m.queryStartedCh != nil {
		m.queryStartedCh <- struct{}{}
	}
	if m.queryDelay > 0 {
		<-time.After(m.queryDelay)
	}
}

func (m *MockPersistentDataStore) Init(allData []intf.StoreCollection) error {
	if m.FakeError != nil {
		return m.FakeError
	}
	for _, mm := range m.Data {
		for k := range mm {
			delete(mm, k)
		}
	}
	for _, coll := range allData {
		itemsMap := make(map[string]intf.VersionedData)
		for _, item := range coll.Items {
			itemsMap[item.GetKey()] = item
		}
		m.Data[coll.Kind] = itemsMap
	}
	*m.inited = true
	return nil
}

func (m *MockPersistentDataStore) Get(kind intf.VersionedDataKind, key string) (intf.VersionedData, error) {
	if m.FakeError != nil {
		return nil, m.FakeError
	}
	m.startQuery()
	if item, ok := m.Data[kind][key]; ok {
		return item, nil
	}
	return nil, nil
}

func (m *MockPersistentDataStore) GetAll(kind intf.VersionedDataKind) (map[string]intf.VersionedData, error) {
	if m.FakeError != nil {
		return nil, m.FakeError
	}
	m.startQuery()
	ret := make(map[string]intf.VersionedData)
	for k, v := range m.Data[kind] {
		ret[k] = v
	}
	return ret, nil
}

func (m *MockPersistentDataStore) Upsert(kind intf.VersionedDataKind, newItem intf.VersionedData) (intf.VersionedData, error) {
	if m.FakeError != nil {
		return nil, m.FakeError
	}
	if m.testTxHook != nil {
		m.testTxHook()
	}
	key := newItem.GetKey()
	if oldItem, ok := m.Data[kind][key]; ok {
		oldVersion := oldItem.GetVersion()
		if oldVersion >= newItem.GetVersion() {
			return oldItem, nil
		}
	}
	m.Data[kind][key] = newItem
	return newItem, nil
}

func (m *MockPersistentDataStore) IsInitialized() bool {
	m.InitQueriedCount++
	return *m.inited
}

func (m *MockPersistentDataStore) IsStoreAvailable() bool {
	return true
}

func (m *MockPersistentDataStore) Close() error {
	m.closed = true
	return nil
}

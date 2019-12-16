package ldtest

import (
	"fmt"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
)

type MockDataItem struct {
	Key     string
	Version int
	Deleted bool
}

func (i *MockDataItem) GetKey() string {
	return i.Key
}

func (i *MockDataItem) GetVersion() int {
	return i.Version
}

func (i *MockDataItem) IsDeleted() bool {
	return i.Deleted
}

type MockDataKind struct{}

var MockData = MockDataKind{}

func (sk MockDataKind) GetNamespace() string {
	return "mock"
}

func (sk MockDataKind) String() string {
	return sk.GetNamespace()
}

func (sk MockDataKind) GetDefaultItem() interface{} {
	return &MockDataItem{}
}

func (sk MockDataKind) MakeDeletedItem(key string, version int) ld.VersionedData {
	return &MockDataItem{Key: key, Version: version, Deleted: true}
}

type MockOtherDataItem struct {
	Key     string
	Version int
	Deleted bool
}

func (i *MockOtherDataItem) GetKey() string {
	return i.Key
}

func (i *MockOtherDataItem) GetVersion() int {
	return i.Version
}

func (i *MockOtherDataItem) IsDeleted() bool {
	return i.Deleted
}

type MockOtherDataKind struct{}

var MockOtherData = MockOtherDataKind{}

func (sk MockOtherDataKind) GetNamespace() string {
	return "mock-other"
}

func (sk MockOtherDataKind) String() string {
	return sk.GetNamespace()
}

func (sk MockOtherDataKind) GetDefaultItem() interface{} {
	return &MockOtherDataItem{}
}

func (sk MockOtherDataKind) MakeDeletedItem(key string, version int) ld.VersionedData {
	return &MockOtherDataItem{Key: key, Version: version, Deleted: true}
}

func makeMockDataMap(items ...ld.VersionedData) map[ld.VersionedDataKind]map[string]ld.VersionedData {
	allData := make(map[ld.VersionedDataKind]map[string]ld.VersionedData)
	for _, item := range items {
		var kind ld.VersionedDataKind
		if _, ok := item.(*MockDataItem); ok {
			kind = MockData
		} else if _, ok := item.(*MockOtherDataItem); ok {
			kind = MockOtherData
		} else {
			panic(fmt.Errorf("unsupported test data type: %T", item))
		}
		items, ok := allData[kind]
		if !ok {
			items = make(map[string]ld.VersionedData)
			allData[kind] = items
		}
		items[item.GetKey()] = item
	}
	return allData
}

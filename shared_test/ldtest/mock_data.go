package ldtest

import (
	"fmt"

	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
)

// MockDataItem is a test implementation of ld.VersionedData.
type MockDataItem struct {
	Key     string
	Version int
	Deleted bool
}

// GetKey returns the item key.
func (i *MockDataItem) GetKey() string {
	return i.Key
}

// GetVersion returns the item version.
func (i *MockDataItem) GetVersion() int {
	return i.Version
}

// IsDeleted returns true if this is a deleted item placeholder.
func (i *MockDataItem) IsDeleted() bool {
	return i.Deleted
}

type mockDataKind struct{}

// MockData is an instance of ld.VersionedDataKind corresponding to MockDataItem.
var MockData = mockDataKind{}

func (sk mockDataKind) GetNamespace() string {
	return "mock1"
}

func (sk mockDataKind) String() string {
	return sk.GetNamespace()
}

func (sk mockDataKind) GetDefaultItem() interface{} {
	return &MockDataItem{}
}

func (sk mockDataKind) MakeDeletedItem(key string, version int) ld.VersionedData {
	return &MockDataItem{Key: key, Version: version, Deleted: true}
}

// MockDataItem is a test implementation of ld.VersionedData.
type MockOtherDataItem struct {
	Key     string
	Version int
	Deleted bool
}

// GetKey returns the item key.
func (i *MockOtherDataItem) GetKey() string {
	return i.Key
}

// GetVersion returns the item version.
func (i *MockOtherDataItem) GetVersion() int {
	return i.Version
}

// IsDeleted returns true if this is a deleted item placeholder.
func (i *MockOtherDataItem) IsDeleted() bool {
	return i.Deleted
}

type mockOtherDataKind struct{}

// MockOtherData is an instance of ld.VersionedDataKind corresponding to MockOtherDataItem.
var MockOtherData = mockOtherDataKind{}

func (sk mockOtherDataKind) GetNamespace() string {
	return "mock2"
}

func (sk mockOtherDataKind) String() string {
	return sk.GetNamespace()
}

func (sk mockOtherDataKind) GetDefaultItem() interface{} {
	return &MockOtherDataItem{}
}

func (sk mockOtherDataKind) MakeDeletedItem(key string, version int) ld.VersionedData {
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

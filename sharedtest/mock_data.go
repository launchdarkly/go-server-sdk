package sharedtest

import (
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

func MakeMockDataSet(items ...*MockDataItem) []intf.StoreCollection {
	itemsColl := intf.StoreCollection{
		Kind:  MockData,
		Items: []intf.VersionedData{},
	}
	otherItemsColl := intf.StoreCollection{
		Kind:  MockOtherData,
		Items: []intf.VersionedData{},
	}
	for _, item := range items {
		if item.IsOtherKind {
			otherItemsColl.Items = append(otherItemsColl.Items, item)
		} else {
			itemsColl.Items = append(itemsColl.Items, item)
		}
	}
	return []intf.StoreCollection{itemsColl, otherItemsColl}
}

// MockDataItem is a test implementation of ld.VersionedData.
type MockDataItem struct {
	Key         string
	Version     int
	Deleted     bool
	Name        string
	IsOtherKind bool
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

func (sk mockDataKind) MakeDeletedItem(key string, version int) interfaces.VersionedData {
	return &MockDataItem{Key: key, Version: version, Deleted: true}
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
	return &MockDataItem{}
}

func (sk mockOtherDataKind) MakeDeletedItem(key string, version int) interfaces.VersionedData {
	return &MockDataItem{Key: key, Version: version, Deleted: true}
}

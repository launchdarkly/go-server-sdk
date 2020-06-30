//nolint:gochecknoglobals,golint,stylecheck
package sharedtest

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
)

// MakeMockDataSet constructs a data set to be passed to a data store's Init method.
func MakeMockDataSet(items ...MockDataItem) []ldstoretypes.Collection {
	itemsColl := ldstoretypes.Collection{
		Kind:  MockData,
		Items: []ldstoretypes.KeyedItemDescriptor{},
	}
	otherItemsColl := ldstoretypes.Collection{
		Kind:  MockOtherData,
		Items: []ldstoretypes.KeyedItemDescriptor{},
	}
	for _, item := range items {
		d := ldstoretypes.KeyedItemDescriptor{
			Key:  item.Key,
			Item: item.ToItemDescriptor(),
		}
		if item.IsOtherKind {
			otherItemsColl.Items = append(otherItemsColl.Items, d)
		} else {
			itemsColl.Items = append(itemsColl.Items, d)
		}
	}
	return []ldstoretypes.Collection{itemsColl, otherItemsColl}
}

// MakeSerializedMockDataSet constructs a data set to be passed to a persistent data store's Init method.
func MakeSerializedMockDataSet(items ...MockDataItem) []ldstoretypes.SerializedCollection {
	itemsColl := ldstoretypes.SerializedCollection{
		Kind:  MockData,
		Items: []ldstoretypes.KeyedSerializedItemDescriptor{},
	}
	otherItemsColl := ldstoretypes.SerializedCollection{
		Kind:  MockOtherData,
		Items: []ldstoretypes.KeyedSerializedItemDescriptor{},
	}
	for _, item := range items {
		d := ldstoretypes.KeyedSerializedItemDescriptor{
			Key:  item.Key,
			Item: item.ToSerializedItemDescriptor(),
		}
		if item.IsOtherKind {
			otherItemsColl.Items = append(otherItemsColl.Items, d)
		} else {
			itemsColl.Items = append(itemsColl.Items, d)
		}
	}
	return []ldstoretypes.SerializedCollection{itemsColl, otherItemsColl}
}

// MockDataItem is a test replacement for FeatureFlag/Segment.
type MockDataItem struct {
	Key         string
	Version     int
	Deleted     bool
	Name        string
	IsOtherKind bool
}

// ToItemDescriptor converts the test item to a StoreItemDescriptor.
func (m MockDataItem) ToItemDescriptor() ldstoretypes.ItemDescriptor {
	return ldstoretypes.ItemDescriptor{Version: m.Version, Item: m}
}

// ToKeyedItemDescriptor converts the test item to a StoreKeyedItemDescriptor.
func (m MockDataItem) ToKeyedItemDescriptor() ldstoretypes.KeyedItemDescriptor {
	return ldstoretypes.KeyedItemDescriptor{Key: m.Key, Item: m.ToItemDescriptor()}
}

// ToSerializedItemDescriptor converts the test item to a StoreSerializedItemDescriptor.
func (m MockDataItem) ToSerializedItemDescriptor() ldstoretypes.SerializedItemDescriptor {
	return ldstoretypes.SerializedItemDescriptor{
		Version:        m.Version,
		Deleted:        m.Deleted,
		SerializedItem: MockData.Serialize(m.ToItemDescriptor()),
	}
}

// MockData is an instance of ld.StoreDataKind corresponding to MockDataItem.
var MockData = mockDataKind{isOther: false}

type mockDataKind struct {
	isOther bool
}

func (sk mockDataKind) GetName() string {
	if sk.isOther {
		return "mock2"
	}
	return "mock1"
}

func (sk mockDataKind) String() string {
	return sk.GetName()
}

func (sk mockDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	if item.Item == nil {
		return []byte(fmt.Sprintf("DELETED:%d", item.Version))
	}
	if mdi, ok := item.Item.(MockDataItem); ok {
		return []byte(fmt.Sprintf("%s,%d,%t,%s,%t", mdi.Key, mdi.Version, mdi.Deleted, mdi.Name, mdi.IsOtherKind))
	}
	return nil
}

func (sk mockDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	if data == nil {
		return ldstoretypes.ItemDescriptor{}.NotFound(), errors.New("tried to deserialize nil data")
	}
	s := string(data)
	if strings.HasPrefix(s, "DELETED:") {
		v, _ := strconv.Atoi(strings.TrimPrefix(s, "DELETED:"))
		return ldstoretypes.ItemDescriptor{Version: v}, nil
	}
	fields := strings.Split(s, ",")
	if len(fields) == 5 {
		v, _ := strconv.Atoi(fields[1])
		itemIsOther := fields[4] == "true"
		if itemIsOther != sk.isOther {
			return ldstoretypes.ItemDescriptor{}.NotFound(), errors.New("got data item of wrong kind")
		}
		isDeleted := fields[2] == "true"
		if isDeleted {
			return ldstoretypes.ItemDescriptor{Version: v}, nil
		}
		m := MockDataItem{Key: fields[0], Version: v, Name: fields[3], IsOtherKind: itemIsOther}
		return ldstoretypes.ItemDescriptor{Version: v, Item: m}, nil
	}
	return ldstoretypes.ItemDescriptor{}.NotFound(), fmt.Errorf(`not a valid MockDataItem: "%s"`, data)
}

// MockOtherData is an instance of ld.StoreDataKind corresponding to another flavor of MockDataItem.
var MockOtherData = mockDataKind{isOther: true}

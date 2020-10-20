package datakinds

import (
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
)

// This file defines the StoreDataKind implementations corresponding to our two top-level data model
// types, FeatureFlag and Segment (both of which are defined in go-server-sdk-evaluation). We access
// these objects directly throughout the SDK. We also export them indirectly in the package
// ldcomponents/ldstoreimpl, because they may be needed by external code that is implementing a
// custom data store.

//nolint:gochecknoglobals // global used as a constant for efficiency
var modelSerialization = ldmodel.NewJSONDataModelSerialization()

// When we produce a JSON representation for a deleted item (tombstone), we need to ensure that it has
// all the properties that the SDKs expect to see in a non-deleted item as well, since some SDKS (like
// PHP) are not tolerant of missing properties when unmarshaling. That means it should have a key as
// well - but the Serialize methods do not receive a key parameter, so we need to make up something.
// The SDK should not actually do anything with the key property in a tombstone but just in case, we'll
// make it something that cannot conflict with a real flag key (since '$' is not allowed).
const deletedItemPlaceholderKey = "$deleted"

// Type aliases for our two implementations of StoreDataKind
type featureFlagStoreDataKind struct{}
type segmentStoreDataKind struct{}

// Features is the global StoreDataKind instance for feature flags.
var Features ldstoretypes.DataKind = featureFlagStoreDataKind{} //nolint:gochecknoglobals

// Segments is the global StoreDataKind instance for segments.
var Segments ldstoretypes.DataKind = segmentStoreDataKind{} //nolint:gochecknoglobals

// AllDataKinds returns all the supported data StoreDataKinds.
func AllDataKinds() []ldstoretypes.DataKind {
	return []ldstoretypes.DataKind{Features, Segments}
}

// GetName returns the unique namespace identifier for feature flag objects.
func (fk featureFlagStoreDataKind) GetName() string {
	return "features"
}

// Serialize is used internally by the SDK when communicating with a PersistentDataStore.
func (fk featureFlagStoreDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	if item.Item == nil {
		flag := ldmodel.FeatureFlag{Key: deletedItemPlaceholderKey, Version: item.Version, Deleted: true}
		if bytes, err := modelSerialization.MarshalFeatureFlag(flag); err == nil {
			return bytes
		}
	} else if flag, ok := item.Item.(*ldmodel.FeatureFlag); ok {
		if bytes, err := modelSerialization.MarshalFeatureFlag(*flag); err == nil {
			return bytes
		}
	}
	return nil
}

// Deserialize is used internally by the SDK when communicating with a PersistentDataStore.
func (fk featureFlagStoreDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	flag, err := modelSerialization.UnmarshalFeatureFlag(data)
	if err != nil {
		return ldstoretypes.ItemDescriptor{}, err
	}
	if flag.Deleted {
		return ldstoretypes.ItemDescriptor{Version: flag.Version, Item: nil}, nil
	}
	return ldstoretypes.ItemDescriptor{Version: flag.Version, Item: &flag}, nil
}

// String returns a human-readable string identifier.
func (fk featureFlagStoreDataKind) String() string {
	return fk.GetName()
}

// GetName returns the unique namespace identifier for segment objects.
func (sk segmentStoreDataKind) GetName() string {
	return "segments"
}

// Serialize is used internally by the SDK when communicating with a PersistentDataStore.
func (sk segmentStoreDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	if item.Item == nil {
		segment := ldmodel.Segment{Key: deletedItemPlaceholderKey, Version: item.Version, Deleted: true}
		if bytes, err := modelSerialization.MarshalSegment(segment); err == nil {
			return bytes
		}
	} else if segment, ok := item.Item.(*ldmodel.Segment); ok {
		if bytes, err := modelSerialization.MarshalSegment(*segment); err == nil {
			return bytes
		}
	}
	return nil
}

// Deserialize is used internally by the SDK when communicating with a PersistentDataStore.
func (sk segmentStoreDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	segment, err := modelSerialization.UnmarshalSegment(data)
	if err != nil {
		return ldstoretypes.ItemDescriptor{}, err
	}
	if segment.Deleted {
		return ldstoretypes.ItemDescriptor{Version: segment.Version, Item: nil}, nil
	}
	return ldstoretypes.ItemDescriptor{Version: segment.Version, Item: &segment}, nil
}

// String returns a human-readable string identifier.
func (sk segmentStoreDataKind) String() string {
	return sk.GetName()
}

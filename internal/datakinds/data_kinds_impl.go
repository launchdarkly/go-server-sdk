package datakinds

import (
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
)

// This file defines the StoreDataKind implementations corresponding to our two top-level data model
// types, FeatureFlag and Segment (both of which are defined in go-server-sdk-evaluation). We access
// these objects directly throughout the SDK. We also export them indirectly in the package
// subsystems/ldstoreimpl, because they may be needed by external code that is implementing a
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
type (
	featureFlagStoreDataKind        struct{}
	segmentStoreDataKind            struct{}
	audienceStoreDataKind           struct{}
	audienceVariationsStoreDataKind struct{}
	variationsStoreDataKind         struct{}
)

// Features is the global StoreDataKind instance for feature flags.
var Features DataKindInternal = featureFlagStoreDataKind{} //nolint:gochecknoglobals

var Audiences DataKindInternal = audienceStoreDataKind{}

var AudienceVariations DataKindInternal = audienceVariationsStoreDataKind{}

var Variations DataKindInternal = variationsStoreDataKind{}

// Segments is the global StoreDataKind instance for segments.
var Segments DataKindInternal = segmentStoreDataKind{} //nolint:gochecknoglobals

// AllDataKinds returns all the supported data StoreDataKinds.
func AllDataKinds() []ldstoretypes.DataKind {
	return []ldstoretypes.DataKind{Features, Segments, Audiences, Variations}
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

func (fk variationsStoreDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	return nil
}

func (fk audienceStoreDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	return nil
}

func (fk audienceVariationsStoreDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	return nil
}

// Deserialize is used internally by the SDK when communicating with a PersistentDataStore.
func (fk featureFlagStoreDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	flag, err := modelSerialization.UnmarshalFeatureFlag(data)
	return maybeFlag(flag, err)
}

// DeserializeFromJSONReader is used internally by the SDK when parsing multiple flags at once.
func (fk featureFlagStoreDataKind) DeserializeFromJSONReader(reader *jreader.Reader) (
	ldstoretypes.ItemDescriptor, error,
) {
	flag := ldmodel.UnmarshalFeatureFlagFromJSONReader(reader)
	return maybeFlag(flag, reader.Error())
}

func maybeFlag(flag ldmodel.FeatureFlag, err error) (ldstoretypes.ItemDescriptor, error) {
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

// GetName returns the unique namespace identifier for segment objects.
func (sk audienceStoreDataKind) GetName() string {
	return "audiences"
}

// GetName returns the unique namespace identifier for audienceVariationStore objects.
func (fk audienceVariationsStoreDataKind) GetName() string {
	return "audienceVariations"
}

// GetName returns the unique namespace identifier for variation objects.
func (sk variationsStoreDataKind) GetName() string {
	return "variations"
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
	return maybeSegment(segment, err)
}

// Deserialize is used internally by the SDK when communicating with a PersistentDataStore.
func (fk audienceStoreDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	return ldstoretypes.ItemDescriptor{}, nil
}

// Deserialize is used internally by the SDK when communicating with a PersistentDataStore.
func (fk audienceVariationsStoreDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	return ldstoretypes.ItemDescriptor{}, nil
}

// Deserialize is used internally by the SDK when communicating with a PersistentDataStore.
func (fk variationsStoreDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	return ldstoretypes.ItemDescriptor{}, nil
}

// DeserializeFromJSONReader is used internally by the SDK when parsing multiple flags at once.
func (fk audienceStoreDataKind) DeserializeFromJSONReader(reader *jreader.Reader) (
	ldstoretypes.ItemDescriptor, error,
) {
	return ldstoretypes.ItemDescriptor{}, nil
}

// DeserializeFromJSONReader is used internally by the SDK when parsing multiple flags at once.
func (fk variationsStoreDataKind) DeserializeFromJSONReader(reader *jreader.Reader) (
	ldstoretypes.ItemDescriptor, error,
) {
	return ldstoretypes.ItemDescriptor{}, nil
}

// DeserializeFromJSONReader is used internally by the SDK when parsing multiple flags at once.
func (fk audienceVariationsStoreDataKind) DeserializeFromJSONReader(reader *jreader.Reader) (
	ldstoretypes.ItemDescriptor, error,
) {
	return ldstoretypes.ItemDescriptor{}, nil
}

// DeserializeFromJSONReader is used internally by the SDK when parsing multiple flags at once.
func (sk segmentStoreDataKind) DeserializeFromJSONReader(reader *jreader.Reader) (ldstoretypes.ItemDescriptor, error) {
	segment := ldmodel.UnmarshalSegmentFromJSONReader(reader)
	return maybeSegment(segment, reader.Error())
}

func maybeSegment(segment ldmodel.Segment, err error) (ldstoretypes.ItemDescriptor, error) {
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

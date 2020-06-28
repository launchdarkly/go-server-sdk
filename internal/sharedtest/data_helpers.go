package sharedtest

import (
	"github.com/launchdarkly/go-test-helpers/ldservices"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// FlagDescriptor is a shortcut for creating a StoreItemDescriptor from a flag.
func FlagDescriptor(f ldmodel.FeatureFlag) interfaces.StoreItemDescriptor {
	return interfaces.StoreItemDescriptor{Version: f.Version, Item: &f}
}

// SegmentDescriptor is a shortcut for creating a StoreItemDescriptor from a segment.
func SegmentDescriptor(s ldmodel.Segment) interfaces.StoreItemDescriptor {
	return interfaces.StoreItemDescriptor{Version: s.Version, Item: &s}
}

// UpsertFlag is a shortcut for calling Upsert with a FeatureFlag.
func UpsertFlag(store interfaces.DataStore, flag *ldmodel.FeatureFlag) {
	_, _ = store.Upsert(interfaces.DataKindFeatures(), flag.Key, FlagDescriptor(*flag))
}

// DataSetBuilder is a helper for creating collections of flags and segments.
type DataSetBuilder struct {
	flags    []interfaces.StoreKeyedItemDescriptor
	segments []interfaces.StoreKeyedItemDescriptor
}

// NewDataSetBuilder creates a DataSetBuilder.
func NewDataSetBuilder() *DataSetBuilder {
	return &DataSetBuilder{}
}

// Build returns the built data sest.
func (d *DataSetBuilder) Build() []interfaces.StoreCollection {
	return []interfaces.StoreCollection{
		interfaces.StoreCollection{Kind: interfaces.DataKindFeatures(), Items: d.flags},
		interfaces.StoreCollection{Kind: interfaces.DataKindSegments(), Items: d.segments},
	}
}

// Flags adds flags to the data set.
func (d *DataSetBuilder) Flags(flags ...ldmodel.FeatureFlag) *DataSetBuilder {
	for _, f := range flags {
		d.flags = append(d.flags, interfaces.StoreKeyedItemDescriptor{Key: f.Key, Item: FlagDescriptor(f)})
	}
	return d
}

// Segments adds segments to the data set.
func (d *DataSetBuilder) Segments(segments ...ldmodel.Segment) *DataSetBuilder {
	for _, s := range segments {
		d.segments = append(d.segments, interfaces.StoreKeyedItemDescriptor{Key: s.Key, Item: SegmentDescriptor(s)})
	}
	return d
}

// ToServerSDKData converts the data set to the format used by the ldservices helpers.
func (d *DataSetBuilder) ToServerSDKData() *ldservices.ServerSDKData {
	ret := ldservices.NewServerSDKData()
	for _, f := range d.flags {
		ret.Flags(f.Item.Item.(*ldmodel.FeatureFlag))
	}
	for _, s := range d.segments {
		ret.Segments(s.Item.Item.(*ldmodel.Segment))
	}
	return ret
}

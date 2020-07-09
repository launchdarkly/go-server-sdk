package sharedtest

import (
	"github.com/launchdarkly/go-test-helpers/v2/ldservices"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
)

// FlagDescriptor is a shortcut for creating a StoreItemDescriptor from a flag.
func FlagDescriptor(f ldmodel.FeatureFlag) ldstoretypes.ItemDescriptor {
	return ldstoretypes.ItemDescriptor{Version: f.Version, Item: &f}
}

// SegmentDescriptor is a shortcut for creating a StoreItemDescriptor from a segment.
func SegmentDescriptor(s ldmodel.Segment) ldstoretypes.ItemDescriptor {
	return ldstoretypes.ItemDescriptor{Version: s.Version, Item: &s}
}

// UpsertFlag is a shortcut for calling Upsert with a FeatureFlag.
func UpsertFlag(store interfaces.DataStore, flag *ldmodel.FeatureFlag) {
	_, _ = store.Upsert(datakinds.Features, flag.Key, FlagDescriptor(*flag))
}

// DataSetBuilder is a helper for creating collections of flags and segments.
type DataSetBuilder struct {
	flags    []ldstoretypes.KeyedItemDescriptor
	segments []ldstoretypes.KeyedItemDescriptor
}

// NewDataSetBuilder creates a DataSetBuilder.
func NewDataSetBuilder() *DataSetBuilder {
	return &DataSetBuilder{}
}

// Build returns the built data sest.
func (d *DataSetBuilder) Build() []ldstoretypes.Collection {
	return []ldstoretypes.Collection{
		ldstoretypes.Collection{Kind: datakinds.Features, Items: d.flags},
		ldstoretypes.Collection{Kind: datakinds.Segments, Items: d.segments},
	}
}

// Flags adds flags to the data set.
func (d *DataSetBuilder) Flags(flags ...ldmodel.FeatureFlag) *DataSetBuilder {
	for _, f := range flags {
		d.flags = append(d.flags, ldstoretypes.KeyedItemDescriptor{Key: f.Key, Item: FlagDescriptor(f)})
	}
	return d
}

// Segments adds segments to the data set.
func (d *DataSetBuilder) Segments(segments ...ldmodel.Segment) *DataSetBuilder {
	for _, s := range segments {
		d.segments = append(d.segments, ldstoretypes.KeyedItemDescriptor{Key: s.Key, Item: SegmentDescriptor(s)})
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

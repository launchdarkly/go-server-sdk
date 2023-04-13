package sharedtest

import (
	"sort"

	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"
	"github.com/launchdarkly/go-server-sdk/v6/testhelpers/ldservices"
)

// FlagDescriptor is a shortcut for creating a StoreItemDescriptor from a flag.
func FlagDescriptor(f ldmodel.FeatureFlag) ldstoretypes.ItemDescriptor {
	return ldstoretypes.ItemDescriptor{Version: f.Version, Item: &f}
}

// SegmentDescriptor is a shortcut for creating a StoreItemDescriptor from a segment.
func SegmentDescriptor(s ldmodel.Segment) ldstoretypes.ItemDescriptor {
	return ldstoretypes.ItemDescriptor{Version: s.Version, Item: &s}
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
		{Kind: datakinds.Features, Items: d.flags},
		{Kind: datakinds.Segments, Items: d.segments},
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

// DataSetToMap converts the data format for Init into a map of maps.
func DataSetToMap(
	allData []ldstoretypes.Collection,
) map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor {
	ret := make(map[ldstoretypes.DataKind]map[string]ldstoretypes.ItemDescriptor, len(allData))
	for _, coll := range allData {
		itemsMap := make(map[string]ldstoretypes.ItemDescriptor, len(coll.Items))
		for _, item := range coll.Items {
			itemsMap[item.Key] = item.Item
		}
		ret[coll.Kind] = itemsMap
	}
	return ret
}

// NormalizeDataSet sorts the data set by kind and key for test determinacy.
func NormalizeDataSet(in []ldstoretypes.Collection) []ldstoretypes.Collection {
	out := append([]ldstoretypes.Collection(nil), in...)
	sort.Slice(out, func(a, b int) bool { return out[a].Kind.GetName() < out[b].Kind.GetName() })
	for i := range out {
		newItems := append([]ldstoretypes.KeyedItemDescriptor(nil), out[i].Items...)
		sort.Slice(newItems, func(a, b int) bool { return newItems[a].Key < newItems[b].Key })
		out[i].Items = newItems
	}
	return out
}

package internal

import (
	"errors"

	"github.com/launchdarkly/go-test-helpers/ldservices"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type dataSetBuilder struct {
	flags    []interfaces.StoreKeyedItemDescriptor
	segments []interfaces.StoreKeyedItemDescriptor
}

func NewDataSetBuilder() *dataSetBuilder {
	return &dataSetBuilder{}
}

func (d *dataSetBuilder) Build() []interfaces.StoreCollection {
	return []interfaces.StoreCollection{
		interfaces.StoreCollection{Kind: interfaces.DataKindFeatures(), Items: d.flags},
		interfaces.StoreCollection{Kind: interfaces.DataKindSegments(), Items: d.segments},
	}
}

func (d *dataSetBuilder) Flags(flags ...ldmodel.FeatureFlag) *dataSetBuilder {
	for _, f := range flags {
		d.flags = append(d.flags, interfaces.StoreKeyedItemDescriptor{Key: f.Key, Item: flagDescriptor(f)})
	}
	return d
}

func (d *dataSetBuilder) Segments(segments ...ldmodel.Segment) *dataSetBuilder {
	for _, s := range segments {
		d.segments = append(d.segments, interfaces.StoreKeyedItemDescriptor{Key: s.Key, Item: segmentDescriptor(s)})
	}
	return d
}

type flagAsSDKData ldmodel.FeatureFlag // hacky type aliasing to let us use ldservices.SDKData with real data model objects
type segmentAsSDKData ldmodel.Segment

func (f flagAsSDKData) GetKey() string    { return f.Key }
func (s segmentAsSDKData) GetKey() string { return s.Key }

func (d *dataSetBuilder) ToServerSDKData() *ldservices.ServerSDKData {
	ret := ldservices.NewServerSDKData()
	for _, f := range d.flags {
		ret.Flags(flagAsSDKData(*(f.Item.Item.(*ldmodel.FeatureFlag))))
	}
	for _, s := range d.segments {
		ret.Segments(segmentAsSDKData(*(s.Item.Item.(*ldmodel.Segment))))
	}
	return ret
}

func flagDescriptor(f ldmodel.FeatureFlag) interfaces.StoreItemDescriptor {
	return interfaces.StoreItemDescriptor{Version: f.Version, Item: &f}
}

func segmentDescriptor(s ldmodel.Segment) interfaces.StoreItemDescriptor {
	return interfaces.StoreItemDescriptor{Version: s.Version, Item: &s}
}

type unknownDataKind struct{}

func (k unknownDataKind) GetName() string {
	return "unknown"
}

func (k unknownDataKind) Serialize(item interfaces.StoreItemDescriptor) []byte {
	return nil
}

func (k unknownDataKind) Deserialize(data []byte) (interfaces.StoreItemDescriptor, error) {
	return interfaces.StoreItemDescriptor{}, errors.New("not implemented")
}

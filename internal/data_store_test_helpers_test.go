package internal

import (
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type dataSetBuilder struct {
	flags    map[string]interfaces.VersionedData
	segments map[string]interfaces.VersionedData
}

func NewDataSetBuilder() *dataSetBuilder {
	return &dataSetBuilder{
		flags:    make(map[string]interfaces.VersionedData),
		segments: make(map[string]interfaces.VersionedData),
	}
}

func (d *dataSetBuilder) Build() map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData {
	return map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
		interfaces.DataKindFeatures(): d.flags,
		interfaces.DataKindSegments(): d.segments,
	}
}

func (d *dataSetBuilder) Flags(flags ...ldmodel.FeatureFlag) *dataSetBuilder {
	for _, f := range flags {
		d.flags[f.Key] = &f
	}
	return d
}

func (d *dataSetBuilder) Segments(segments ...ldmodel.Segment) *dataSetBuilder {
	for _, s := range segments {
		d.segments[s.Key] = &s
	}
	return d
}

type unknownDataKind struct{}

func (k unknownDataKind) GetNamespace() string {
	return "unknown"
}

func (k unknownDataKind) GetDefaultItem() interface{} {
	return nil
}

func (k unknownDataKind) MakeDeletedItem(key string, version int) interfaces.VersionedData {
	return nil
}

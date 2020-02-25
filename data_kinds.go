package ldclient

import (
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// VersionedDataKinds is a list of supported VersionedDataKind's. Among other things, this list might
// be used by data stores to know what data (namespaces) to expect.
var VersionedDataKinds = [...]interfaces.VersionedDataKind{
	Features,
	Segments,
}

// featureFlagVersionedDataKind implements VersionedDataKind and provides methods to build storage engine for flags.
type featureFlagVersionedDataKind struct{}

// GetNamespace returns the a unique namespace identifier for feature flag objects
func (fk featureFlagVersionedDataKind) GetNamespace() string {
	return "features"
}

// String returns the namespace
func (fk featureFlagVersionedDataKind) String() string {
	return fk.GetNamespace()
}

// GetDefaultItem returns a default feature flag representation
func (fk featureFlagVersionedDataKind) GetDefaultItem() interface{} {
	return &ldmodel.FeatureFlag{}
}

// MakeDeletedItem returns representation of a deleted flag
func (fk featureFlagVersionedDataKind) MakeDeletedItem(key string, version int) interfaces.VersionedData {
	return &ldmodel.FeatureFlag{Key: key, Version: version, Deleted: true}
}

// Features is a convenience variable to access an instance of VersionedDataKind.
//
// Deprecated: this variable is for internal use and will be removed in a future version.
var Features interfaces.VersionedDataKind = featureFlagVersionedDataKind{}

// segmentVersionedDataKind implements VersionedDataKind and provides methods to build storage engine for segments.
type segmentVersionedDataKind struct{}

// GetNamespace returns the a unique namespace identifier for feature flag objects
func (sk segmentVersionedDataKind) GetNamespace() string {
	return "segments"
}

// String returns the namespace
func (sk segmentVersionedDataKind) String() string {
	return sk.GetNamespace()
}

// GetDefaultItem returns a default segment representation
func (sk segmentVersionedDataKind) GetDefaultItem() interface{} {
	return &ldmodel.Segment{}
}

// MakeDeletedItem returns representation of a deleted segment
func (sk segmentVersionedDataKind) MakeDeletedItem(key string, version int) interfaces.VersionedData {
	return &ldmodel.Segment{Key: key, Version: version, Deleted: true}
}

// Segments is a convenience variable to access an instance of VersionedDataKind.
//
// Deprecated: this variable is for internal use and will be moved to another package in a future version.
var Segments interfaces.VersionedDataKind = segmentVersionedDataKind{}

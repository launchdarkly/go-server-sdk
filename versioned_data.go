package ldclient

import ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"

// VersionedData is a common interface for string-keyed, versioned objects such as feature flags.
type VersionedData interface {
	// GetKey returns the string key for this object.
	GetKey() string
	// GetVersion returns the version number for this object.
	GetVersion() int
	// IsDeleted returns whether or not this object has been deleted.
	IsDeleted() bool
}

// VersionedDataKind describes a kind of VersionedData objects that may exist in a store.
type VersionedDataKind interface {
	// GetNamespace returns a short string that serves as the unique name for the collection of these objects, e.g. "features".
	GetNamespace() string
	// GetDefaultItem return a pointer to a newly created null value of this object type. This is used for JSON unmarshalling.
	GetDefaultItem() interface{}
	// MakeDeletedItem returns a value of this object type with the specified key and version, and Deleted=true.
	MakeDeletedItem(key string, version int) VersionedData
}

// VersionedDataKinds is a list of supported VersionedDataKind's. Among other things, this list might
// be used by data stores to know what data (namespaces) to expect.
var VersionedDataKinds = [...]VersionedDataKind{
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
	return &ldeval.FeatureFlag{}
}

// MakeDeletedItem returns representation of a deleted flag
func (fk featureFlagVersionedDataKind) MakeDeletedItem(key string, version int) VersionedData {
	return &ldeval.FeatureFlag{Key: key, Version: version, Deleted: true}
}

// Features is a convenience variable to access an instance of VersionedDataKind.
//
// Deprecated: this variable is for internal use and will be removed in a future version.
var Features VersionedDataKind = featureFlagVersionedDataKind{}

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
	return &ldeval.Segment{}
}

// MakeDeletedItem returns representation of a deleted segment
func (sk segmentVersionedDataKind) MakeDeletedItem(key string, version int) VersionedData {
	return &ldeval.Segment{Key: key, Version: version, Deleted: true}
}

// Segments is a convenience variable to access an instance of VersionedDataKind.
//
// Deprecated: this variable is for internal use and will be moved to another package in a future version.
var Segments VersionedDataKind = segmentVersionedDataKind{}

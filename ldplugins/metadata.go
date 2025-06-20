package ldplugins

// Metadata contains information about a specific plugin implementation.
type Metadata struct {
	name string
}

// MetadataOption represents a functional means of setting additional, optional, attributes of the Metadata.
type MetadataOption func(metadata *Metadata)

// Implementation note: Currently the plugin metadata only contains a name, but it may contain additional, and likely
// optional, fields in the future. The MetadataOption will allow for additional options to be added without
// breaking callsites.
//
// Example:
// NewMetadata("my-plugin", WithVendorName("LaunchDarkly"))
//

// NewMetadata creates Metadata with the provided name.
func NewMetadata(name string, opts ...MetadataOption) Metadata {
	metadata := Metadata{
		name: name,
	}
	for _, opt := range opts {
		opt(&metadata)
	}
	return metadata
}

// Name gets the name of the plugin implementation.
func (m Metadata) Name() string {
	return m.name
}

// SdkMetadata contains metadata about the SDK that is running the plugin.
type SdkMetadata struct {
	// The name of the SDK.
	Name string
	// The version of the SDK.
	Version string
	// If this is a wrapper SDK, then this is the name of the wrapper.
	WrapperName string
	// If this is a wrapper SDK, then this is the version of the wrapper.
	WrapperVersion string
}

// ApplicationMetadata contains metadata about the application where the LaunchDarkly SDK is running.
type ApplicationMetadata struct {
	// A unique identifier representing the application where the LaunchDarkly SDK is running.
	ID string
	// A unique identifier representing the version of the application where the LaunchDarkly SDK is running.
	Version string
}

// EnvironmentMetadata contains metadata about the environment where the plugin is running.
type EnvironmentMetadata struct {
	// Metadata about the SDK that is running the plugin.
	Sdk SdkMetadata
	// SDK key that was used to initialize the client.
	SdkKey string
	// Metadata about the application where the LaunchDarkly SDK is running.
	Application ApplicationMetadata
}

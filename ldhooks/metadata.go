package ldhooks

// Metadata contains information about a specific hook implementation.
type Metadata struct {
	name string
}

// HookMetadataOption represents a functional means of setting additional, optional, attributes of the Metadata.
type HookMetadataOption func(hook *Metadata)

// Implementation note: Currently the hook metadata only contains a name, but it may contain additional, and likely
// optional, fields in the future. The HookMetadataOption will allow for additional options to be added without
// breaking callsites.
//
// Example:
// NewMetadata("my-hook", WithVendorName("LaunchDarkly"))
//

// NewMetadata creates Metadata with the provided name.
func NewMetadata(name string, opts ...HookMetadataOption) Metadata {
	metadata := Metadata{
		name: name,
	}
	for _, opt := range opts {
		opt(&metadata)
	}
	return metadata
}

// Name gets the name of the hook implementation.
func (m Metadata) Name() string {
	return m.name
}

package ldhooks

// HookMetadata contains information about a specific hook implementation.
type HookMetadata struct {
	name string
}

// HookMetadataOption represents a functional means of setting additional, optional, attributes of the HookMetadata.
type HookMetadataOption func(hook *HookMetadata)

// Implementation note: Currently the hook metadata only contains a name, but it may contain additional, and likely
// optional, fields in the future. The HookMetadataOption will allow for additional options to be added without
// breaking callsites.
//
// Example:
// NewHookMetadata("my-hook", WithVendorName("LaunchDarkly"))
//

// NewHookMetadata creates HookMetadata with the provided name.
func NewHookMetadata(name string, opts ...HookMetadataOption) HookMetadata {
	metadata := HookMetadata{
		name: name,
	}
	for _, opt := range opts {
		opt(&metadata)
	}
	return metadata
}

// Name gets the name of the hook implementation.
func (m HookMetadata) Name() string {
	return m.name
}

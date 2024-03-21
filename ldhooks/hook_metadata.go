package ldhooks

// HookMetadata contains information about a specific hook implementation.
type HookMetadata struct {
	name string
}

// NewHookMetadata creates HookMetadata with the provided name.
func NewHookMetadata(name string) HookMetadata {
	return HookMetadata{
		name: name,
	}
}

func (m HookMetadata) GetName() string {
	return m.name
}

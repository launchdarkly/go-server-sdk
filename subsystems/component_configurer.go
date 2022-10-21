package subsystems

// ComponentConfigurer is a common interface for SDK component factories and configuration builders.
// Applications should not need to implement this interface.
type ComponentConfigurer[T any] interface {
	// Build is called internally by the SDK to create an implementation instance. Applications
	// should not need to call this method.
	Build(clientContext ClientContext) (T, error)
}

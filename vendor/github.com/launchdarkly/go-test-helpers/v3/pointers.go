package helpers

// AsPointer returns a pointer to a copy of a value.
func AsPointer[V any](v V) *V {
	return &v
}

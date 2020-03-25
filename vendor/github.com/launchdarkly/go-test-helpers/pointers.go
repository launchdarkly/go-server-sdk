package helpers

// BoolPtr returns a pointer to a bool value. This is useful within expressions where the value is a literal.
func BoolPtr(b bool) *bool {
	return &b
}

// IntPtr returns a pointer to an int value. This is useful within expressions where the value is a literal.
func IntPtr(n int) *int {
	return &n
}

// Float64Ptr returns a pointer to an float64 value. This is useful within expressions where the value is a literal.
func Float64Ptr(n float64) *float64 {
	return &n
}

// StrPtr returns a pointer to a string value. This is useful within expressions where the value is a literal.
func StrPtr(s string) *string {
	return &s
}

// Uint64Ptr returns a pointer to a uint64 value. This is useful within expressions where the value is a literal.
func Uint64Ptr(n uint64) *uint64 {
	return &n
}

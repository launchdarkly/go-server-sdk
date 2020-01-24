package ldvalue

// OptionalString represents a string that may or may not have a value. This is similar to using a
// string pointer to distinguish between an empty string and nil, but it is safer because it does
// not expose a pointer to any mutable value.
//
// Unlike Value, which can contain a value of any JSON-compatible type, OptionalString either
// contains a string or nothing.
type OptionalString struct {
	value    string
	hasValue bool
}

// NewOptionalStringWithValue constructs an OptionalString that has a string value.
func NewOptionalStringWithValue(value string) OptionalString {
	return OptionalString{value: value, hasValue: true}
}

// NewOptionalStringFromPointer constructs an OptionalString from a string pointer. If the pointer
// is non-nil, then the OptionalString copies its value; otherwise the OptionalString is empty.
func NewOptionalStringFromPointer(valuePointer *string) OptionalString {
	if valuePointer == nil {
		return OptionalString{hasValue: false}
	}
	return OptionalString{value: *valuePointer, hasValue: true}
}

// IsDefined returns true if the OptionalString contains a string value, or false if it is empty.
func (o OptionalString) IsDefined() bool {
	return o.hasValue
}

// StringValue returns the OptionalString's value, or an empty string if it has no value.
func (o OptionalString) StringValue() string {
	return o.value
}

// AsPointer returns the OptionalString's value as a string pointer if it has a value (copying the
// value, rather than returning a pointer to the internal field), or nil if it has no value.
func (o OptionalString) AsPointer() *string {
	if o.hasValue {
		s := o.value
		return &s
	}
	return nil
}

// AsValue converts the OptionalString to a Value, which is either Null() or a string value.
func (o OptionalString) AsValue() Value {
	if o.hasValue {
		return String(o.value)
	}
	return Null()
}

// String is a debugging convenience method that returns a description of the OptionalString.
// This is either the same as its string value, "[empty]" if it has a string value that is empty,
// or "[none]" if it has no value.
func (o OptionalString) String() string {
	if o.hasValue {
		if o.value == "" {
			return "[empty]"
		}
		return o.value
	}
	return "[none]"
}

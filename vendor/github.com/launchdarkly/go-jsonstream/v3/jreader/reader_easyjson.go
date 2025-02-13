//go:build launchdarkly_easyjson
// +build launchdarkly_easyjson

package jreader

// String attempts to read a string value.
//
// If there is a parsing error, or the next value is not a string, the return value is "" and
// the Reader enters a failed state, which you can detect with Error(). Types other than string
// are never converted to strings.
func (r *Reader) String() string {
	r.awaitingReadValue = false
	if r.err != nil {
		return ""
	}
	val, err := r.tr.String()
	if err != nil {
		r.err = err
		return ""
	}
	return val
}

// StringAsBytes attempts to read a string value, returning the string as a byte slice.
//
// If there is a parsing error, or the next value is not a string, the return value is nil and
// the Reader enters a failed state, which you can detect with Error(). Types other than string
// are never converted to strings.
func (r *Reader) StringAsBytes() []byte {
	return []byte(r.String())
}

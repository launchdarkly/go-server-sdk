//go:build !launchdarkly_easyjson
// +build !launchdarkly_easyjson

package jreader

// String attempts to read a string value.
//
// If there is a parsing error, or the next value is not a string, the return value is "" and
// the Reader enters a failed state, which you can detect with Error(). Types other than string
// are never converted to strings.
func (r *Reader) String() string {
	return string(r.StringAsBytes())
}

// StringAsBytes attempts to read a string value, returning a byte slice that indexes into the
// original JSON bytes.  This method can be used instead of String to avoid garbage creation,
// but care must be taken to avoid modifying the returned byte slice.
//
// If there is a parsing error, or the next value is not a string, the return value is nil and
// the Reader enters a failed state, which you can detect with Error(). Types other than string
// are never converted to strings.
func (r *Reader) StringAsBytes() []byte {
	r.awaitingReadValue = false
	if r.err != nil {
		return nil
	}
	val, err := r.tr.StringAsBytes()
	if err != nil {
		r.err = err
		return nil
	}
	return val
}

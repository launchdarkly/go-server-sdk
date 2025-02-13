package jsonhelpers

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// JValue is a helper type for manipulating JSON data in tests. It validates that marshaled
// data is valid JSON, allows other data to be converted to JSON, and eliminates ambiguity
// as to whether a type like string or []byte in a test represents JSON or not.
type JValue struct {
	raw    string
	parsed any
	err    error
}

// String returns the JSON value as a string.
func (v JValue) String() string {
	return v.raw
}

// Error returns nil if the value is valid JSON, or else an error value describing the problem.
func (v JValue) Error() error {
	return v.err
}

// Equal returns true if the values are deeply equal.
func (v JValue) Equal(v1 JValue) bool {
	if v.err != nil || v1.err != nil {
		return v.err == v1.err
	}
	return reflect.DeepEqual(v.parsed, v1.parsed)
}

// JValueOf creates a JValue based on any input type, as follows:
//
// - If the input type is []byte, json.RawMessage, or string, it interprets the value as JSON.
// - If the input type is JValue, it returns the same value.
// - For any other type, it attempts to marshal the value to JSON.
//
// If the input value is invalid, the returned JValue will have a non-nil Error().
func JValueOf(value any) JValue {
	var data []byte
	switch v := value.(type) {
	case JValue:
		return v
	case json.RawMessage:
		data = v
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		d, err := json.Marshal(value)
		if err != nil {
			return JValue{
				raw:    "<invalid>",
				parsed: value,
				err:    fmt.Errorf("value could not be marshalled to JSON: %s", err),
			}
		}
		data = d
	}
	var intf interface{}
	if err := json.Unmarshal(data, &intf); err != nil {
		return JValue{
			raw:    string(data),
			parsed: nil,
			err:    fmt.Errorf("JSON unmarshaling error: %s", err),
		}
	}
	return JValue{raw: string(data), parsed: intf, err: nil}
}

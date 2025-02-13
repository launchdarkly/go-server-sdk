package matchers

import (
	"fmt"
	"reflect"
)

// Equal is a matcher that tests whether the input value matches the expected value according
// to reflect.DeepEqual, except in the case of numbers where an exact type match is not needed.
func Equal(expectedValue interface{}) Matcher {
	return New(
		func(value interface{}) bool {
			return reflect.DeepEqual(canonicalizeValue(value), canonicalizeValue(expectedValue))
		},
		func() string {
			return fmt.Sprintf("equal to %s", DescribeValue(expectedValue))
		},
		func(value interface{}) string {
			return fmt.Sprintf("did not equal %s", DescribeValue(expectedValue))
		},
	)
}

// BeNil is a matcher that passes if the value is a nil interface value, a nil pointer, a
// nil slice, or a nil map.
func BeNil() Matcher {
	return New(
		func(value interface{}) bool {
			if value == nil {
				return true
			}
			rv := reflect.ValueOf(value)
			switch rv.Type().Kind() {
			case reflect.Ptr, reflect.Slice, reflect.Map:
				return rv.IsNil()
			}
			return false
		},
		func() string {
			return "is nil"
		},
		func(value interface{}) string {
			return "was not nil"
		},
	)
}

// Automatic numeric type conversion for use with Equals(), to avoid the common problem of
// expecting for instance int(1) in a JSON data structure which parsed it as float64(1).
func canonicalizeValue(value interface{}) interface{} {
	switch v := value.(type) {
	case uint8:
		return uint64(v)
	case uint:
		return uint64(v)
	case int8:
		return float64(v)
	case int:
		return float64(v)
	case float32:
		return float64(v)
	}
	return value
}

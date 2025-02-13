package matchers

import (
	"fmt"
	"reflect"
)

// MatcherTransform is a combinator that allows an input value to be transformed to some
// other value (possibly of a different type) before being tested by other Matchers.
//
// For instance, this could be used to access a field inside a struct or some other nested
// data structure. Assuming there is a struct type S with a field F, you could do this:
//
//	SF := matchers.Transform("F",
//	    func(value interface{}) interface{} { return value.(S).F })
//	SF.Should(Equal(3)).Assert(t, someInstanceOfS)
//
// The advantages of doing this, instead of simply getting the F field directly and
// testing it, are 1. you can use combinators such as AllOf to test multiple properties
// in a single assertion, and 2. failure messages will automatically include both a full
// description of someInstanceOfS and an explanation of what was wrong with it. For
// instance, in the example above, if someInstanceOfS.F was really 4, the failure message
// would show:
//
//	expected: F equal to 3
//	actual value was: {F: 4}
//
// You can use MatcherTransform's other methods to add type safety.
type MatcherTransform struct {
	name         string
	getValue     func(interface{}) (interface{}, error)
	expectedType interface{}
}

// Transform creates a MatcherTransform. The name parameter is a brief description of what
// the output value is in relation to the input value (for instance, if you are getting a
// field called F from a struct, it could simply be "F"); it will be prefixed to the
// description of any Matcher that you use with Should(). The getValue parameter is a
// function that transforms the original value into the value you will be testing.
func Transform(
	name string,
	getValue func(interface{}) (interface{}, error),
) MatcherTransform {
	return MatcherTransform{name: name, getValue: getValue}
}

// EnsureInputValueType is the equivalent of Matcher.EnsureValueType. Given any value of
// the desired type, it returns a modified MatcherTransform that will safely fail if the
// wrong type is passed in.
//
//	stringLength := matchers.Transform("string length",
//	    func(value interface{}) interface{} { return len(value.(string)) }).
//	    EnsureInputValueType("")
func (mt MatcherTransform) EnsureInputValueType(valueOfType interface{}) MatcherTransform {
	mt.expectedType = valueOfType
	return mt
}

// Should applies a Matcher to the transformed value. That is, assuming that this MatcherTransform
// converts an A value into a B value, mt.Should(Equal(3)) returns a Matcher that takes A,
// converts it to B, and applies Equal(3) to B.
func (mt MatcherTransform) Should(matcher Matcher) Matcher {
	if mt.getValue == nil {
		mt.getValue = func(value interface{}) (interface{}, error) { return value, nil }
	}
	return New(
		func(value interface{}) bool {
			newValue, err := mt.getValue(value)
			if err != nil {
				return false
			}
			return matcher.test(newValue)
		},
		func() string {
			ret := mt.name
			if ret != "" {
				ret += " "
			}
			return ret + matcher.describeTest()
		},
		func(value interface{}) string {
			newValue, err := mt.getValue(value)
			if err != nil {
				return err.Error()
			}
			ret := mt.name
			if ret != "" {
				ret += " "
			}
			return ret + matcher.describeFailure(newValue)
		},
	).EnsureType(mt.expectedType)
}

// Length is a MatcherTransform that takes any value len() can operate on, gets its
// length, and applies some matcher to the result.
func Length() MatcherTransform {
	return Transform(
		"length",
		func(value interface{}) (interface{}, error) {
			v := reflect.ValueOf(value)
			t := v.Type()
			switch t.Kind() {
			case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
				return v.Len(), nil
			default:
				return nil, fmt.Errorf("matchers.Length() was used for an inapplicable type (%T)", value)
			}
		},
	)
}

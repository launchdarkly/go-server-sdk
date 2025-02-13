package matchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/launchdarkly/go-test-helpers/v3/jsonhelpers"
)

// JSONEqual is similar to Equal but with richer behavior for JSON values.
//
// Both the expected value and the actual value can be of any type. If the type is either []byte
// or json.RawMessage, it will be interpreted as JSON which will be parsed; for all other types,
// it will be first serialized to JSON with json.Marshal and then parsed. Then the parsed values
// or data structures are tested for deep equality. For instance, this test passes:
//
//	matchers.In(t).Assert([]byte(`{"a": true, "b": false`),
//	    matchers.JSONEqual(map[string]bool{b: false, a: true}))
//
// The shortcut JSONEqualStr can be used to avoid writing []byte() if the expected value is
// already a serialized JSON string.
func JSONEqual(expectedValue interface{}) Matcher {
	expectedIntf, expectedValueErr := toJSONInterface(expectedValue)
	return New(
		func(value interface{}) bool {
			if expectedValueErr != nil {
				return false
			}
			valueIntf, err := toJSONInterface(value)
			if err != nil {
				return false
			}
			return reflect.DeepEqual(valueIntf, expectedIntf)
		},
		func() string {
			return fmt.Sprintf("JSON equal to %s", jsonhelpers.CanonicalizeJSON(jsonhelpers.ToJSON(expectedIntf)))
		},
		func(value interface{}) string {
			if expectedValueErr != nil {
				return fmt.Sprintf("bad expected value in assertion (%s)", expectedValueErr)
			}
			valueIntf, err := toJSONInterface(value)
			if err != nil {
				return err.Error()
			}
			diff, err := jsonhelpers.JSONDiff(jsonhelpers.ToJSON(expectedIntf), jsonhelpers.ToJSON(valueIntf))
			if err != nil {
				return err.Error()
			}
			if len(diff) == 1 && diff[0].Path == nil {
				return fmt.Sprintf("expected: JSON equal to %s", diff[0].Value1)
			}
			return "JSON values " + strings.Join(diff.Describe("expected", "actual"), "\n")
		},
	)
}

// JSONStrEqual is equivalent to JSONEqual except that it converts expectedValue from string
// to []byte first, and if the input value is a string it does the same. This is convenient if
// you are matching against already-serialized JSON, because otherwise passing a string value
// to JSONEqual would cause that value to be serialized in the way JSON represents strings,
// that is, with quoting and escaping.
//
//	matchers.In(t).Assert(`{"a": true, "b": false`,
//	    matchers.JSONStrEqual(`{"b": false, "a": true}`)
func JSONStrEqual(expectedValue string) Matcher {
	return Transform("", func(value interface{}) (interface{}, error) {
		if s, ok := value.(string); ok {
			return []byte(s), nil
		}
		return value, nil
	}).Should(JSONEqual([]byte(expectedValue)))
}

// JSONProperty is a MatcherTransform that takes any value serializable as a JSON object
// and gets a named property from it; then you can apply a matcher to the value of that
// property. It fails if no such property exists (see OptJSONProperty).
//
//	myObject := []byte(`{"a": {"b": 2}}`)
//	matchers.In(t).Assert(myObject,
//	    matchers.JSONProperty("a").Should(
//	        matchers.JSONProperty("b").Should(Equal(2))))
//
// An alternative is to use JSONMap combined with MapOf or MapIncluding.
func JSONProperty(name string) MatcherTransform {
	return Transform(
		fmt.Sprintf("JSON property %q", name),
		func(value interface{}) (interface{}, error) {
			m, err := toJSONObjectMap(value)
			if err != nil {
				return nil, err
			}
			if propValue, ok := m[name]; ok {
				return propValue, nil
			}
			return nil, fmt.Errorf("JSON property %q not found", name)
		},
	)
}

// JSONOptProperty is the same as JSONProperty, but if the property does not exist, it treats it
// as a nil value rather than error.
func JSONOptProperty(name string) MatcherTransform {
	return Transform(
		fmt.Sprintf("JSON property %q", name),
		func(value interface{}) (interface{}, error) {
			m, err := toJSONObjectMap(value)
			if err != nil {
				return nil, err
			}
			return m[name], nil
		},
	)
}

// JSONArray is a MatcherTransform that takes any value serializable as a JSON array, and converts
// it to []interface{} slice; then you can apply a matcher to that slice. It fails if the value is
// not serializable as a JSON array.
//
//	myArray := []byte(`["a", "b", "c"]`)
//	matchers.In(t).Assert(myArray,
//	    matchers.JSONArray().Should(matchers.Length().Should(matchers.Equal(3))))
func JSONArray() MatcherTransform {
	return Transform(
		"JSON array",
		func(value interface{}) (interface{}, error) {
			v, err := toJSONInterface(value)
			if err != nil {
				return nil, err
			}
			if s, ok := v.([]interface{}); ok {
				return s, nil
			}
			return nil, errors.New("wanted a JSON array but found a different type")
		},
	)
}

// JSONMap is a MatcherTransform that takes any value serializable as a JSON object, and converts
// it to a map[interface{}]interface{}; then you can apply a matcher to that map. It fails if the
// value is not serializable as a JSON object.
//
//	myArray := []byte(`{"a": 1, "b": "xyz"}`)
//	matchers.In(t).Assert(myJSON,
//	    matchers.JSONMap().Should(
//	        matchers.MapOf(
//	            matchers.KV("a", matchers.Equal(1)),
//	            matchers.KV("b", matchers.StringHasPrefix("x")),
//	        )))
func JSONMap() MatcherTransform {
	return Transform(
		"JSON map",
		func(value interface{}) (interface{}, error) {
			m, err := toJSONObjectMap(value)
			if err != nil {
				return nil, err
			}
			return m, nil
		},
	)
}

func toJSONInterface(value interface{}) (interface{}, error) {
	var data []byte
	switch v := value.(type) {
	case json.RawMessage:
		data = v
	case []byte:
		data = v
	default:
		d, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("value could not be marshalled to JSON: %s", err)
		}
		data = d
	}
	var intf interface{}
	if err := json.Unmarshal(data, &intf); err != nil {
		return nil, fmt.Errorf("value was not valid JSON: %s", err)
	}
	return intf, nil
}

func toJSONObjectMap(value interface{}) (map[string]interface{}, error) {
	valueIntf, err := toJSONInterface(value)
	if err != nil {
		return nil, err
	}
	if m, ok := valueIntf.(map[string]interface{}); ok {
		return m, nil
	}
	if s, ok := valueIntf.(string); ok {
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(s), &m); err == nil {
				return m, nil
			}
		}
	}
	return nil, errors.New("wanted a JSON object but found a different type")
}

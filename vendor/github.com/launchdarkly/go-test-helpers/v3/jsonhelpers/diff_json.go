package jsonhelpers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// JSONDiffResult is a list of JSONDiffElement values returned by JSONDiff.
type JSONDiffResult []JSONDiffElement

// Describe returns a list of string descriptions of the differences.
func (r JSONDiffResult) Describe(value1Name, value2Name string) []string {
	ret := make([]string, 0, len(r))
	for _, e := range r {
		ret = append(ret, e.Describe(value1Name, value2Name))
	}
	return ret
}

// JSONDiffElement describes a point of difference between two JSON data structures.
type JSONDiffElement struct {
	// Path represents the location of the data as a path from the root.
	Path JSONPath

	// Value1 is the JSON encoding of the value at that path in the data structure
	// that was passed to JSONDiff as json1. An empty string (as opposed to the JSON
	// representation of an empty string, `""`) means that this property was missing
	// in json1.
	Value1 string

	// Value2 is the JSON encoding of the value at that path in the data structure
	// that was passed to JSONDiff as json2. An empty string (as opposed to the JSON
	// representation of an empty string, `""`) means that this property was missing
	// in json2.
	Value2 string
}

// Describe returns a string description of this difference.
func (e JSONDiffElement) Describe(value1Name, value2Name string) string {
	var desc1, desc2 = e.Value1, e.Value2
	if desc1 == "" {
		desc1 = "<absent>"
	}
	if desc2 == "" {
		desc2 = "<absent>"
	}
	pathPrefix := ""
	if len(e.Path) != 0 {
		pathPrefix = fmt.Sprintf("at %s: ", e.Path)
	}
	return fmt.Sprintf("%s%s = %s, %s = %s", pathPrefix, value1Name, desc1, value2Name, desc2)
}

// JSONPath represents the location of a node in a JSON data structure.
//
// In a JSON object {"a":{"b":2}}, the nested "b":2 property would be referenced as
// JSONPath{{Property: "a"}, {Property: "b"}}.
//
// In a JSON array ["a","b",["c"]], the "c" value would be referenced as
// JSONPath{{Index: 2},{Index: 0}}.
//
// A nil or zero-length slice represents the root of the data.
type JSONPath []JSONPathComponent

// String returns a string representation of the path.
func (p JSONPath) String() string {
	parts := make([]string, 0, len(p))
	for _, c := range p {
		if c.Property == "" {
			parts = append(parts, fmt.Sprintf("[%d]", c.Index))
		} else {
			parts = append(parts, fmt.Sprintf(`"%s"`, c.Property))
		}
	}
	return strings.Join(parts, ".")
}

// JSONPathComponent represents a location within the top level of a JSON object or array.
type JSONPathComponent struct {
	// Property is the name of an object property, or "" if this is in an array.
	Property string

	// Index is the zero-based index of an array element, if this is in an array.
	Index int
}

// JSONDiff compares two JSON values and returns an explanation of how they differ, if at all,
// ignoring any differences that do not affect the value semantically (such as whitespace).
// This is for programmatic use; if you want a human-readable test assertion based on the
// same logic, see matchers.JSONEqual (which calls this function).
//
// The two values are provided as marshalled JSON data. If they cannot be parsed, the
// function immediately returns an error.
//
// If the values are deeply equal, the result is nil.
//
// Otherwise, if they are both simple values, the result will contain a single
// JSONDiffElement.
//
// If they are both JSON objects, JSONDiff will compare their properties. It will produce
// a JSONDiffElement for each property where they differ. For instance, comparing
// {"a": 1, "b": 2} with {"a": 1, "b": 3, "c": 4} will produce one element for "b" and
// one for "c". If a property contains an object value on both sides, the comparison will
// proceed recursively and may produce elements with subpaths (see JSONPath).
//
// If they are both JSON arrays, and are of the same length, JSONDiff will compare their
// elements using the same rules as above. For JSON arrays of different lengths, if the
// shorter one matches every corresponding element of the longer one, it will return a
// JSONDiffElement pointing to the first element after the shorter one and listing the
// additional elements starting with a comma (for instance, comparing [10,20] with
// [10,20,30] will return a string of ",30" at index 2); otherwise it will just return
// both arrays in their entirety.
//
// Values that are not of the same type will always produce a single JSONDiffElement
// describing the entire values.
func JSONDiff(json1, json2 []byte) (JSONDiffResult, error) {
	var value1, value2 interface{}
	if err := json.Unmarshal(json1, &value1); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(json2, &value2); err != nil {
		return nil, err
	}
	return describeValueDifference(value1, value2, nil), nil
}

func describeValueDifference(value1, value2 interface{}, path JSONPath) JSONDiffResult {
	if a1, ok := value1.([]interface{}); ok {
		if a2, ok := value2.([]interface{}); ok {
			return describeArrayValueDifference(a1, a2, path)
		}
	}
	if o1, ok := value1.(map[string]interface{}); ok {
		if o2, ok := value2.(map[string]interface{}); ok {
			return describeObjectValueDifference(o1, o2, path)
		}
	}
	if reflect.DeepEqual(value1, value2) {
		return nil
	}
	return JSONDiffResult{
		{Path: path, Value1: ToJSONString(value1), Value2: ToJSONString(value2)},
	}
}

func describeArrayValueDifference(array1, array2 []interface{}, path JSONPath) JSONDiffResult {
	if len(array1) != len(array2) {
		// Check for the case where one is a shorter version of the other but the same up to that point
		if len(array1) != 0 && len(array2) != 0 {
			shortestCommonLength := len(array1)
			if shortestCommonLength > len(array2) {
				shortestCommonLength = len(array2)
			}
			foundUnequal := false
			for i := 0; i < shortestCommonLength; i++ {
				if !reflect.DeepEqual(array1[i], array2[i]) {
					foundUnequal = true
					break
				}
			}
			if !foundUnequal {
				var remainder []interface{}
				if len(array1) == shortestCommonLength {
					remainder = array2[shortestCommonLength:]
				} else {
					remainder = array1[shortestCommonLength:]
				}
				remainderStr := ToJSONString(remainder)
				remainderStr = "," + strings.TrimSuffix(strings.TrimPrefix(remainderStr, "["), "]")
				ret := JSONDiffElement{
					Path: append(append(JSONPath(nil), path...), JSONPathComponent{Index: shortestCommonLength}),
				}
				if len(array1) == shortestCommonLength {
					ret.Value2 = remainderStr
				} else {
					ret.Value1 = remainderStr
				}
				return JSONDiffResult{ret}
			}
		}
		return JSONDiffResult{
			{Path: path, Value1: ToJSONString(array1), Value2: ToJSONString(array2)},
		}
	}

	var diffs JSONDiffResult
	for i, value1 := range array1 {
		subpath := append(append(JSONPath(nil), path...), JSONPathComponent{Index: i})
		value2 := array2[i]
		diffs = append(diffs, describeValueDifference(value1, value2, subpath)...)
	}

	return diffs
}

func describeObjectValueDifference(object1, object2 map[string]interface{}, path JSONPath) JSONDiffResult {
	allKeys := make(map[string]struct{})
	for key := range object1 {
		allKeys[key] = struct{}{}
	}
	for key := range object2 {
		allKeys[key] = struct{}{}
	}
	allSortedKeys := make([]string, 0, len(allKeys))
	for key := range allKeys {
		allSortedKeys = append(allSortedKeys, key)
	}
	sort.Strings(allSortedKeys)

	var diffs JSONDiffResult //nolint:prealloc

	for _, key := range allSortedKeys {
		subpath := append(append(JSONPath(nil), path...), JSONPathComponent{Property: key})

		var desc1, desc2 = "", ""
		if value1, ok := object1[key]; ok {
			if value2, ok := object2[key]; ok {
				if reflect.DeepEqual(value1, value2) {
					continue
				}
				diffs = append(diffs, describeValueDifference(value1, value2, subpath)...)
				continue
			} else {
				desc1 = string(CanonicalizeJSON(ToJSON(value1)))
			}
		} else {
			desc2 = string(CanonicalizeJSON(ToJSON(object2[key])))
		}
		diffs = append(diffs, JSONDiffElement{
			Path:   subpath,
			Value1: desc1,
			Value2: desc2,
		})
	}
	return diffs
}

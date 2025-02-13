package matchers

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// KeyValueMatcher is used with MapOf or MapIncluding to describe a matcher for a key-value pair in a map.
type KeyValueMatcher struct {
	Key   interface{}
	Value Matcher
}

// KV is a shortcut for constructing a KeyValueMatcher for use with MapOf or MapIncluding.
func KV(key interface{}, valueMatcher Matcher) KeyValueMatcher {
	return KeyValueMatcher{Key: key, Value: valueMatcher}
}

// Items is a matcher for a slice or array value. It tests that the number of elements is equal to
// the number of matchers, and that each element matches the corresponding matcher in order.
//
//	s := []int{6,2}
//	matchers.Items(matchers.Equal(6), matchers.Equal(2)).Test(s) // pass
//	matchers.Items(matchers.Equal(2), matchers.Equal(6)).Test(s) // fail
func Items(matchers ...Matcher) Matcher {
	return New(
		func(value interface{}) bool {
			elements, err := getSliceOrArrayElementValues(value)
			if err != nil || len(elements) != len(matchers) {
				return false
			}
			for i, m := range matchers {
				elementValue := elements[i]
				if !m.test(elementValue) {
					return false
				}
			}
			return true
		},
		func() string {
			return "items: " + describeMatchers(matchers, "")
		},
		func(value interface{}) string {
			elements, err := getSliceOrArrayElementValues(value)
			if err != nil {
				return err.Error()
			}
			if len(elements) != len(matchers) {
				return fmt.Sprintf("expected slice with %d item(s), got %d item(s)", len(matchers), len(elements))
			}
			parts := make([]string, 0, len(matchers))
			for i, m := range matchers {
				elementValue := elements[i]
				if !m.test(elementValue) {
					parts = append(parts, fmt.Sprintf("item[%d] %s", i, m.describeFailure(elementValue)))
				}
			}
			return strings.Join(parts, ", ")
		},
	)
}

// ItemsInAnyOrder is a matcher for a slice or array value. It tests that the number of elements is
// equal to the number of matchers, and that each matcher matches an element.
//
//	s := []int{6,2}
//	matchers.ItemsInAnyOrder(matchers.Equal(2), matchers.Equal(6)).Test(s) // pass
func ItemsInAnyOrder(matchers ...Matcher) Matcher {
	return New(
		func(value interface{}) bool {
			elements, err := getSliceOrArrayElementValues(value)
			if err != nil || len(elements) != len(matchers) {
				return false
			}
			foundCount := 0
			for _, m := range matchers {
				for _, elementValue := range elements {
					if m.test(elementValue) {
						foundCount++
						break
					}
				}
			}
			return foundCount == len(matchers)
		},
		func() string {
			return "items in any order: " + describeMatchers(matchers, ", ")
		},
		func(value interface{}) string {
			// Describing a failure for ItemsInAnyOrder requires us to repeat the matching logic we
			// previously executed, but in a bit more detail. For any matcher that successfully
			// matched an item, we don't need to describe that matcher or that item.
			elements, err := getSliceOrArrayElementValues(value)
			if err != nil {
				return err.Error()
			}
			type unmatchedElement struct {
				index int
				value interface{}
			}
			unmatchedElements := make([]unmatchedElement, 0)
			for i, e := range elements {
				unmatchedElements = append(unmatchedElements, unmatchedElement{index: i, value: e})
			}
			unmatchedMatchers := append([]Matcher(nil), matchers...)
			for i := 0; i < len(unmatchedMatchers); i++ {
				m := unmatchedMatchers[i]
				for j := 0; j < len(unmatchedElements); j++ {
					if m.test(unmatchedElements[j].value) {
						unmatchedElements = append(unmatchedElements[0:j], unmatchedElements[j+1:]...)
						unmatchedMatchers = append(unmatchedMatchers[0:i], unmatchedMatchers[i+1:]...)
						j--
						i--
						break
					}
				}
			}
			if len(unmatchedMatchers) == 0 {
				// Every matcher matched a value but there were some values left over.
				parts := make([]string, 0)
				for _, e := range unmatchedElements {
					parts = append(parts, fmt.Sprintf("[%d]: %s", e.index, DescribeValue(e.value)))
				}
				return fmt.Sprintf("got more items than expected: %s", strings.Join(parts, ", "))
			}
			if len(unmatchedMatchers) == 1 && len(unmatchedElements) == 1 {
				// In this case we'll assume that this was the element that was supposed to be matched
				// by this matcher, but its value was wrong somehow. So we'll print the failure message
				// from the matcher.
				return fmt.Sprintf("failed expectation for one item [%d] with value: %s\nfailure was: %s",
					unmatchedElements[0].index, DescribeValue(unmatchedElements[0].value),
					unmatchedMatchers[0].describeFailure(unmatchedElements[0].value))
			}
			// If there was more than one unmatched matcher and/or element, we can't really guess
			// which one was supposed to go with which, so we'll just print all the unmet conditions.
			return fmt.Sprintf("no items were found to match: %s", describeMatchers(unmatchedMatchers, ", "))
		},
	)
}

// MapOf is a matcher for a map value. It tests that the map has exactly the same keys as the
// specified list, and that the matcher for each key is satisfied by the corresponding value.
//
//	m := map[string]int{"a": 6, "b": 2}
//	matchers.MapOf(
//	    matchers.KV("a", matchers.Equal(2)),
//	    matchers.KV("b", matchers.Equal(6)),
//	}).Test(s) // pass
func MapOf(keyValueMatchers ...KeyValueMatcher) Matcher {
	return New(
		func(value interface{}) bool {
			valueAsMap, err := getMapValues(value)
			if err != nil || len(valueAsMap) != len(keyValueMatchers) {
				return false
			}
			for _, kv := range keyValueMatchers {
				if elementValue, ok := valueAsMap[kv.Key]; ok {
					if !kv.Value.test(elementValue) {
						return false
					}
				} else {
					return false
				}
			}
			return true
		},
		func() string {
			var parts []string
			for _, kv := range keyValueMatchers {
				parts = append(parts, fmt.Sprintf("%s: %s", kv.Key, kv.Value.describeTest()))
			}
			return "map: {" + strings.Join(parts, ", ") + "}"
		},
		func(value interface{}) string {
			valueAsMap, err := getMapValues(value)
			if err != nil {
				return err.Error()
			}
			if len(valueAsMap) != len(keyValueMatchers) {
				return fmt.Sprintf("expected map keys %v but got map keys %v", getSortedExpectedKeys(keyValueMatchers),
					getSortedMapKeys(valueAsMap))
			}
			parts := make([]string, 0, len(keyValueMatchers))
			for _, kv := range keyValueMatchers {
				if elementValue, ok := valueAsMap[kv.Key]; ok {
					if !kv.Value.test(elementValue) {
						parts = append(parts, fmt.Sprintf("key [%s] %s", kv.Key, kv.Value.describeFailure(elementValue)))
					}
				} else {
					parts = append(parts, fmt.Sprintf("key [%s] not found", kv.Key))
				}
			}
			return strings.Join(parts, ", ")
		},
	)
}

// MapIncluding is a matcher for a map value. It tests that the map contains all of the keys in
// the specified list, and that the matcher for each key is satisfied by the corresponding value.
// The map may also contain additional keys.
//
//	m := map[string]int{"a": 6, "b": 2}
//	matchers.MapOf(
//	    matchers.KV("a", matchers.Equal(2)),
//	    matchers.KV("b", matchers.Equal(6)),
//	}).Test(s) // pass
func MapIncluding(keyValueMatchers ...KeyValueMatcher) Matcher {
	return New(
		func(value interface{}) bool {
			valueAsMap, err := getMapValues(value)
			if err != nil {
				return false
			}
			for _, kv := range keyValueMatchers {
				if elementValue, ok := valueAsMap[kv.Key]; ok {
					if !kv.Value.test(elementValue) {
						return false
					}
				} else {
					return false
				}
			}
			return true
		},
		func() string {
			var parts []string
			for _, kv := range keyValueMatchers {
				parts = append(parts, fmt.Sprintf("%s: %s", kv.Key, kv.Value.describeTest()))
			}
			return "map including: {" + strings.Join(parts, ", ") + "}"
		},
		func(value interface{}) string {
			valueAsMap, err := getMapValues(value)
			if err != nil {
				return err.Error()
			}
			parts := make([]string, 0, len(keyValueMatchers))
			for _, kv := range keyValueMatchers {
				if elementValue, ok := valueAsMap[kv.Key]; ok {
					if !kv.Value.test(elementValue) {
						parts = append(parts, fmt.Sprintf("key [%s] %s", kv.Key, kv.Value.describeFailure(elementValue)))
					}
				} else {
					parts = append(parts, fmt.Sprintf("key [%s] not found", kv.Key))
				}
			}
			return strings.Join(parts, ", ")
		},
	)
}

func getSliceOrArrayElementValues(sliceValue interface{}) ([]interface{}, error) {
	v := reflect.ValueOf(sliceValue)
	if v.Type().Kind() != reflect.Slice && v.Type().Kind() != reflect.Array {
		return nil, fmt.Errorf("expected slice or array value but got %T", sliceValue)
	}
	ret := make([]interface{}, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		ret = append(ret, v.Index(i).Interface())
	}
	return ret, nil
}

func getMapValues(mapValue interface{}) (map[interface{}]interface{}, error) {
	v := reflect.ValueOf(mapValue)
	if v.Type().Kind() != reflect.Map {
		return nil, fmt.Errorf("expected map value but got %T", mapValue)
	}
	ret := make(map[interface{}]interface{}, v.Len())
	for _, k := range v.MapKeys() {
		ret[k.Interface()] = v.MapIndex(k).Interface()
	}
	return ret, nil
}

func getSortedExpectedKeys(keyValueMatchers []KeyValueMatcher) []string {
	ret := make([]string, 0, len(keyValueMatchers))
	for _, kv := range keyValueMatchers {
		ret = append(ret, fmt.Sprintf("%v", kv.Key))
	}
	sort.Strings(ret)
	return ret
}

func getSortedMapKeys(mapValue interface{}) []string {
	v := reflect.ValueOf(mapValue)
	if v.Type().Kind() != reflect.Map {
		return nil
	}
	ret := make([]string, 0, v.Len())
	for _, k := range v.MapKeys() {
		ret = append(ret, fmt.Sprintf("%v", k.Interface()))
	}
	sort.Strings(ret)
	return ret
}

// ValueForKey is a MatcherTransform that takes a map, looks up a value in it by key,
// and applies a matcher to that value. It fails if no such key exists (see
// OptValueForKey).
//
//	myMap := map[string]map[string]int{"a": map[string]int{"b": 2}}
//	matchers.In(t).Assert(myObject,
//	    matchers.ValueForKey("a").Should(
//	        matchers.ValueForKey("b").Should(Equal(2))))
func ValueForKey(key interface{}) MatcherTransform {
	return Transform(
		fmt.Sprintf("for key %s", DescribeValue(key)),
		func(value interface{}) (interface{}, error) {
			if value == nil {
				return nil, errors.New("map was nil")
			}
			rv := reflect.ValueOf(value)
			if rv.Type().Kind() != reflect.Map {
				return nil, fmt.Errorf("expected a map but got %T", value)
			}
			for _, k := range rv.MapKeys() {
				if k.Interface() == key {
					return rv.MapIndex(k).Interface(), nil
				}
			}
			return nil, fmt.Errorf("map key %s not found", DescribeValue(key))
		},
	)
}

// OptValueForKey is a MatcherTransform that takes a map, looks up a value in it by key,
// and applies a matcher to that value. If no such key exists, it returns the zero
// value for the type. If the map was nil, it returns nil.
//
//	myMap := map[string]map[string]int{"a": map[string]int{"b": 2}}
//	matchers.In(t).Assert(myMap,
//	    matchers.OptValueForKey("a").Should(
//	        matchers.OptValueForKey("c").Should(Equal(0))))
func OptValueForKey(key interface{}) MatcherTransform {
	return Transform(
		fmt.Sprintf("for key %s", DescribeValue(key)),
		func(value interface{}) (interface{}, error) {
			if value == nil {
				return nil, nil
			}
			rv := reflect.ValueOf(value)
			if rv.Type().Kind() != reflect.Map {
				return nil, fmt.Errorf("expected a map but got %T", value)
			}
			result := rv.MapIndex(reflect.ValueOf(key))
			if !result.IsValid() {
				return reflect.Zero(rv.Type().Elem()).Interface(), nil
			}
			return result.Interface(), nil
		},
	)
}

package matchers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/launchdarkly/go-test-helpers/v3/jsonhelpers"
)

// DescribeValue tries to create attractive string representations of values for test
// failure messages. The logic is as follows (whichever comes first):
//
// If the value is nil, it returns "nil".
//
// If the type is a struct that has "json" field tags, it is converted to JSON.
//
// If the type implements fmt.Stringer, its String method is called.
//
// If the type is string, it is quoted, unless it already has bracket or brace delimiters.
//
// If the type is []byte, it is converted to a string unchanged, unless it is valid JSON
// in which case it is passed to jsonhelpers.CanonicalizeJSON.
//
// If the type is json.RawMessage, it is passed to jsonhelpers.CanonicalizeJSON.
//
// If the type is a slice or array, it is formatted as [value1, value2, value3] (unlike
// Go's default formatting which has no commas) and each value is recursively formatted
// with DescribeValue.
//
// At last resort, it is formatted with fmt.Sprintf("%+v").
func DescribeValue(value interface{}) string {
	if value == nil {
		return "nil"
	}
	if isJSONTaggedStruct(value) {
		return string(jsonhelpers.CanonicalizeJSON(jsonhelpers.ToJSON(value)))
	}
	switch v := value.(type) {
	case fmt.Stringer:
		return v.String()
	case string:
		if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
			return v
		}
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			return v
		}
		return `"` + v + `"`
	case []byte:
		return string(jsonhelpers.CanonicalizeJSON(v))
	case json.RawMessage:
		return string(jsonhelpers.CanonicalizeJSON(v))
	default:
		rv := reflect.ValueOf(value)
		if rv.Type().Kind() == reflect.Array || rv.Type().Kind() == reflect.Slice {
			parts := make([]string, 0, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				parts = append(parts, DescribeValue(rv.Index(i).Interface()))
			}
			return "[" + strings.Join(parts, ", ") + "]"
		}
		return fmt.Sprintf("%+v", value)
	}
}

func isJSONTaggedStruct(value interface{}) bool {
	t := reflect.TypeOf(value)
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue // field is not exported
		}
		tagStr := field.Tag.Get("json")
		if tagStr != "" {
			return true
		}
	}
	return false
}

func describeMatchers(matchers []Matcher, separator string) string {
	if len(matchers) == 1 {
		return matchers[0].describeTest()
	}
	parts := make([]string, 0, len(matchers))
	for _, m := range matchers {
		parts = append(parts, "("+m.describeTest()+")")
	}
	return strings.Join(parts, separator)
}

func describeFailures(matchers []Matcher, value interface{}) string {
	var fails []string
	for _, m := range matchers {
		if !m.test(value) {
			fails = append(fails, m.describeFailure(value))
		}
	}
	return strings.Join(fails, ", ")
}

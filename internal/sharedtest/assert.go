package sharedtest

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertNotNil forces a panic if the specified value is nil (either a nil interface value, or a
// nil pointer).
func AssertNotNil(i interface{}) {
	if i != nil {
		val := reflect.ValueOf(i)
		if val.Kind() != reflect.Ptr || !val.IsNil() {
			return
		}
	}
	panic("unexpected nil pointer or nil interface value")
}

// AssertValuesJSONEqual serializes both values to JSON and calls assert.JSONEq.
func AssertValuesJSONEqual(t *testing.T, expected interface{}, actual interface{}) {
	bytes1, _ := json.Marshal(expected)
	bytes2, _ := json.Marshal(actual)
	assert.JSONEq(t, string(bytes1), string(bytes2))
}

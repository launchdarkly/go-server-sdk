package ldclient

import (
	"reflect"
	"strconv"
	"time"
	"encoding/json"
	"fmt"
)

// Converts any of the following into a pointer to a time.Time value:
//   RFC3339/ISO8601 timestamp (example: 2006-01-02T15:04:05.999Z07:00)
//   Unix epoch milliseconds as string
//   Unix milliseconds as number
// Passing in a time.Time value will return a pointer to the input value.
// Unparsable inputs will return nil
// More info on RFC3339: http://stackoverflow.com/questions/522251/whats-the-difference-between-iso-8601-and-rfc-3339-date-formats
func parseTime(input interface{}) *time.Time {
	if input == nil {
		return nil
	}

	// First check if we can easily detect the type as a time.Time or timestamp as string
	switch typedInput := input.(type) {
	case time.Time:
		return &typedInput
	case string:
		value, err := time.Parse(time.RFC3339Nano, typedInput)
		if err == nil {
			utcValue := value.UTC()
			return &utcValue
		}
	}

	// Is it a number or can it be parsed as a number?
	parsedNumberPtr := parseFloat64(input)
	if parsedNumberPtr != nil {
		value := unixMillisToUtcTime(*parsedNumberPtr)
		return &value
	}
	return nil
}

// Parses numeric value as float64 from a string or another numeric type.
// Returns nil pointer if input is nil or unparsable.
func parseFloat64(input interface{}) *float64 {
	if input == nil {
		return nil
	}

	switch typedInput := input.(type) {
	case float64:
		return &typedInput
	case string:
		inputFloat64, err := strconv.ParseFloat(typedInput, 64)
		if err == nil {
			return &inputFloat64
		}
	default:
		float64Type := reflect.TypeOf(float64(0))
		v := reflect.ValueOf(input)
		v = reflect.Indirect(v)
		if v.Type().ConvertibleTo(float64Type) {
			floatValue := v.Convert(float64Type)
			f64 := floatValue.Float()
			return &f64
		}
	}
	return nil
}

// Converts a number of any type to an int.
func numberToInt(input interface{}) (int, error) {
	if input == nil {
		return 0, nil
	}

	switch typedInput := input.(type) {
	case int:
		return typedInput, nil
	default:
		intType := reflect.TypeOf(int(0))
		v := reflect.ValueOf(input)
		v = reflect.Indirect(v)
		if v.Type().ConvertibleTo(intType) {
			intValue := v.Convert(intType)
			return int(intValue.Int()), nil
		}
	}
	return 0, fmt.Errorf("Could not convert: %+v to int!", input)

}

// Convert a Unix epoch milliseconds float64 value to the equivalent time.Time value with UTC location
func unixMillisToUtcTime(unixMillis float64) time.Time {
	return time.Unix(0, int64(unixMillis)*int64(time.Millisecond)).UTC()
}

// Converts input to a *json.RawMessage if possible.
func toJsonRawMessage(input interface{}) (json.RawMessage, error) {
	if input == nil {
		return nil, nil
	}
	switch typedInput := input.(type) {
	//already json, so just return casted input value
	case json.RawMessage:
		return typedInput, nil
	case []byte:
		inputJsonRawMessage := json.RawMessage(typedInput)
		return inputJsonRawMessage, nil
	default:
		inputJsonBytes, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("Could not marshal: %+v to json", input)
		}
		inputJsonRawMessage := json.RawMessage(inputJsonBytes)
		return inputJsonRawMessage, nil
	}
	return nil, fmt.Errorf("Could not convert: %+v to json.RawMessage", input)
}

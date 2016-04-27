package ldclient

import (
	"reflect"
	"strconv"
	"time"
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
	parsedNumberPtr := parseNumber(input)
	if parsedNumberPtr != nil {
		value := unixMillisToUtcTime(*parsedNumberPtr)
		return &value
	}
	return nil
}

// Parses numeric value as float64 from a string or another numeric type.
// Returns nil pointer if input is nil or unparsable.
func parseNumber(input interface{}) *float64 {
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

// Convert a Unix epoch milliseconds float64 value to the equivalent time.Time value with UTC location
func unixMillisToUtcTime(unixMillis float64) time.Time {
	return time.Unix(0, int64(unixMillis)*1000000).UTC()
}

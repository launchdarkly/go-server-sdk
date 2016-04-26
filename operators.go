package ldclient

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	operatorIn                 Operator = "in"
	operatorEndsWith           Operator = "endsWith"
	operatorStartsWith         Operator = "startsWith"
	operatorMatches            Operator = "matches"
	operatorContains           Operator = "contains"
	operatorLessThan           Operator = "lessThan"
	operatorLessThanOrEqual    Operator = "lessThanOrEqual"
	operatorGreaterThan        Operator = "greaterThan"
	operatorGreaterThanOrEqual Operator = "greaterThanOrEqual"
	operatorBefore             Operator = "before"
	operatorAfter              Operator = "after"
)

type opFn (func(interface{}, interface{}) bool)

type Operator string

var allOps = map[Operator]opFn{
	operatorIn:                 operatorInFn,
	operatorEndsWith:           operatorEndsWithFn,
	operatorStartsWith:         operatorStartsWithFn,
	operatorMatches:            operatorMatchesFn,
	operatorContains:           operatorContainsFn,
	operatorLessThan:           operatorLessThanFn,
	operatorLessThanOrEqual:    operatorLessThanOrEqualFn,
	operatorGreaterThan:        operatorGreaterThanFn,
	operatorGreaterThanOrEqual: operatorGreaterThanOrEqualFn,
	operatorBefore:             operatorBeforeFn,
	operatorAfter:              operatorAfterFn,
}

// Turn this into a static map
func operatorFn(operator Operator) opFn {
	if op, ok := allOps[operator]; ok {
		return op
	} else {
		return operatorNoneFn
	}
}

func operatorInFn(uValue interface{}, cValue interface{}) bool {
	if uValue == cValue {
		return true
	}

	if numericOperator(uValue, cValue, func(u float64, c float64) bool { return u == c }) {
		return true
	}

	uTime := parseTime(uValue)
	if uTime != nil {
		cTime := parseTime(cValue)
		if cTime != nil {
			return uTime.Equal(*cTime)
		}
	}
	return false
}

func stringOperator(uValue interface{}, cValue interface{}, fn func(string, string) bool) bool {
	if uStr, ok := uValue.(string); ok {
		if cStr, ok := cValue.(string); ok {
			return fn(uStr, cStr)
		}
	}
	return false

}

func operatorStartsWithFn(uValue interface{}, cValue interface{}) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool { return strings.HasPrefix(u, c) })
}

func operatorEndsWithFn(uValue interface{}, cValue interface{}) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool { return strings.HasSuffix(u, c) })
}

func operatorMatchesFn(uValue interface{}, cValue interface{}) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool {
		if matched, err := regexp.MatchString(c, u); err == nil {
			return matched
		} else {
			return false
		}
	})
}

func operatorContainsFn(uValue interface{}, cValue interface{}) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool { return strings.Contains(u, c) })
}

func numericOperator(uValue interface{}, cValue interface{}, fn func(float64, float64) bool) bool {
	uFloat64 := parseNumber(uValue)
	if uFloat64 != nil {
		cFloat64 := parseNumber(cValue)
		if cFloat64 != nil {
			return fn(*uFloat64, *cFloat64)
		}
	}
	return false
}

func operatorLessThanFn(uValue interface{}, cValue interface{}) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u < c })
}

func operatorLessThanOrEqualFn(uValue interface{}, cValue interface{}) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u <= c })
}

func operatorGreaterThanFn(uValue interface{}, cValue interface{}) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u > c })
}

func operatorGreaterThanOrEqualFn(uValue interface{}, cValue interface{}) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u >= c })
}

func operatorBeforeFn(uValue interface{}, cValue interface{}) bool {
	uTime := parseTime(uValue)
	if uTime != nil {
		cTime := parseTime(cValue)
		if cTime != nil {
			return uTime.Before(*cTime)
		}
	}
	return false
}

func operatorAfterFn(uValue interface{}, cValue interface{}) bool {
	uTime := parseTime(uValue)
	if uTime != nil {
		cTime := parseTime(cValue)
		if cTime != nil {
			return uTime.After(*cTime)
		}
	}
	return false
}

func operatorNoneFn(uValue interface{}, cValue interface{}) bool {
	return false
}

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

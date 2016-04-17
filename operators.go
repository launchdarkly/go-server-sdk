package ldclient

import (
	"errors"
	"fmt"
	_ "fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	//RFC3339Millis = "2006-01-02T15:04:05.999Z07:00"
	operatorIn          Operator = "in"
	operatorEndsWith    Operator = "endsWith"
	operatorStartsWith  Operator = "startsWith"
	operatorMatches     Operator = "matches"
	operatorContains    Operator = "contains"
	operatorLessThan    Operator = "lessThan"
	operatorGreaterThan Operator = "greaterThan"
	operatorBefore      Operator = "before"
	operatorAfter       Operator = "after"
	operatorWithin      Operator = "within"
)

type opFn (func(interface{}, interface{}) bool)

type Operator string

var allOps = map[Operator]opFn{
	operatorIn:          operatorInFn,
	operatorEndsWith:    operatorEndsWithFn,
	operatorStartsWith:  operatorStartsWithFn,
	operatorMatches:     operatorMatchesFn,
	operatorContains:    operatorContainsFn,
	operatorLessThan:    operatorLessThanFn,
	operatorGreaterThan: operatorGreaterThanFn,
	//operatorBefore:      operatorBeforeFn, //dates accept string or number, then parse as either epoch millis or ISO8601
	//operatorAfter:       operatorAfterFn,
	//operatorWithin:      operatorWithinFn,
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
	return uValue == cValue
}

func operatorStartsWithFn(uValue interface{}, cValue interface{}) bool {
	if uStr, ok := uValue.(string); ok {
		if cStr, ok := cValue.(string); ok {
			return strings.HasPrefix(uStr, cStr)
		}
	}
	return false
}

func operatorEndsWithFn(uValue interface{}, cValue interface{}) bool {
	if uStr, ok := uValue.(string); ok {
		if cStr, ok := cValue.(string); ok {
			return strings.HasSuffix(uStr, cStr)
		}
	}
	return false
}

func operatorMatchesFn(uValue interface{}, cValue interface{}) bool {
	if uStr, ok := uValue.(string); ok {
		if pattern, ok := cValue.(string); ok {
			if matched, err := regexp.MatchString(pattern, uStr); err == nil {
				return matched
			} else {
				return false
			}
		}
	}
	return false
}

func operatorContainsFn(uValue interface{}, cValue interface{}) bool {
	if uStr, ok := uValue.(string); ok {
		if cStr, ok := cValue.(string); ok {
			return strings.Contains(uStr, cStr)
		}
	}
	return false
}

func operatorLessThanFn(uValue interface{}, cValue interface{}) bool {
	if uFloat64, ok := uValue.(float64); ok {
		if cFloat64, ok := cValue.(float64); ok {
			return uFloat64 < cFloat64
		}
	}
	return false
}

func operatorGreaterThanFn(uValue interface{}, cValue interface{}) bool {
	if uFloat64, ok := uValue.(float64); ok {
		if cFloat64, ok := cValue.(float64); ok {
			return uFloat64 > cFloat64
		}
	}
	return false
}

func operatorBeforeFn(uValue interface{}, cValue interface{}) bool {
	if uFloat64, ok := uValue.(float64); ok {
		//we got epoch millis

		if cFloat64, ok := cValue.(float64); ok {
			return uFloat64 > cFloat64
		}
	}
	return false
}

func operatorNoneFn(uValue interface{}, cValue interface{}) bool {
	return false
}

// Converts any of the following into a time.Time value:
//   RFC3339 timestamp (example: 2006-01-02T15:04:05.999Z07:00)
//   Unix epoch milliseconds as string
//   Unix milliseconds as number
// Passing in a time.Time value will result in returning the same value.
// More info on RFC3339: http://stackoverflow.com/questions/522251/whats-the-difference-between-iso-8601-and-rfc-3339-date-formats
func parseTime(input interface{}) (time.Time, error) {
	if input == nil {
		return time.Time{}, errors.New("Cannot parse nil value as date")
	}

	switch typedInput := input.(type) {
	case time.Time:
		return typedInput, nil
	case string:
		// stringified number?
		inputFloat64, err := strconv.ParseFloat(typedInput, 64)
		if err == nil {
			return unixMillisToUtcTime(inputFloat64), nil
		}
		// timestamp?
		value, err := time.Parse(time.RFC3339Nano, typedInput)
		if err != nil {
			return time.Time{}, err
		}
		return value.UTC(), nil
	default:
		// is it numeric?
		float64Type := reflect.TypeOf(float64(0))
		v := reflect.ValueOf(input)
		v = reflect.Indirect(v)
		if v.Type().ConvertibleTo(float64Type) {
			floatValue := v.Convert(float64Type)
			return unixMillisToUtcTime(floatValue.Float()), nil
		}
		return time.Time{}, errors.New(fmt.Sprintf("Could not parse value: %+v as unix epoch millis or RFC3339 timestamp", input))
	}
}

// Convert a Unix epoch milliseconds float64 value to the equivalent time.Time value with UTC location
func unixMillisToUtcTime(unixMillis float64) time.Time {
	return time.Unix(0, int64(unixMillis)*1000000).UTC()
}

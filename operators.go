package ldclient

import (
	"regexp"
	"strings"
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

	uTime := ParseTime(uValue)
	if uTime != nil {
		cTime := ParseTime(cValue)
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
	uFloat64 := ParseFloat64(uValue)
	if uFloat64 != nil {
		cFloat64 := ParseFloat64(cValue)
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
	uTime := ParseTime(uValue)
	if uTime != nil {
		cTime := ParseTime(cValue)
		if cTime != nil {
			return uTime.Before(*cTime)
		}
	}
	return false
}

func operatorAfterFn(uValue interface{}, cValue interface{}) bool {
	uTime := ParseTime(uValue)
	if uTime != nil {
		cTime := ParseTime(cValue)
		if cTime != nil {
			return uTime.After(*cTime)
		}
	}
	return false
}

func operatorNoneFn(uValue interface{}, cValue interface{}) bool {
	return false
}
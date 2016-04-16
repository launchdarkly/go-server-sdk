package ldclient

import (
	"regexp"
	"strings"
)

const (
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

func operatorNoneFn(uValue interface{}, cValue interface{}) bool {
	return false
}

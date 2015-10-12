package ldclient

import (
	"regexp"
	"strings"
)

const (
	operatorIn         = "in"
	operatorEndsWith   = "endsWith"
	operatorStartsWith = "startsWith"
	operatorMatches    = "matches"
)

type opFn (func(interface{}, interface{}) bool)

func operatorFn(operator Operator) opFn {
	switch operator {
	case operatorIn:
		return operatorInFn
	case operatorEndsWith:
		return operatorEndsWithFn
	case operatorStartsWith:
		return operatorStartsWithFn
	case operatorMatches:
		return operatorMatchesFn
	default:
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

func operatorNoneFn(uValue interface{}, cValue interface{}) bool {
	return false
}

package ldclient

import (
	"strings"
)

type opFn (func(interface{}, interface{}) bool)

func operatorFn(operator Operator) opFn {
	switch operator {
	case "in":
		return operatorIn
	case "endsWith":
		return operatorEndsWith
	case "startsWith":
		return operatorStartsWith
	default:
		return operatorNone
	}
}

func operatorIn(uValue interface{}, cValue interface{}) bool {
	return uValue == cValue
}

func operatorStartsWith(uValue interface{}, cValue interface{}) bool {
	if uStr, ok := uValue.(string); ok {
		if cStr, ok := cValue.(string); ok {
			return strings.HasPrefix(uStr, cStr)
		}
	}
	return false
}

func operatorEndsWith(uValue interface{}, cValue interface{}) bool {
	if uStr, ok := uValue.(string); ok {
		if cStr, ok := cValue.(string); ok {
			return strings.HasSuffix(uStr, cStr)
		}
	}
	return false
}

func operatorNone(uValue interface{}, cValue interface{}) bool {
	return false
}

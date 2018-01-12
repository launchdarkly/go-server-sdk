package ldclient

import (
	"regexp"
	"strings"
	
	"github.com/blang/semver"
)

const (
	OperatorIn                 Operator = "in"
	OperatorEndsWith           Operator = "endsWith"
	OperatorStartsWith         Operator = "startsWith"
	OperatorMatches            Operator = "matches"
	OperatorContains           Operator = "contains"
	OperatorLessThan           Operator = "lessThan"
	OperatorLessThanOrEqual    Operator = "lessThanOrEqual"
	OperatorGreaterThan        Operator = "greaterThan"
	OperatorGreaterThanOrEqual Operator = "greaterThanOrEqual"
	OperatorBefore             Operator = "before"
	OperatorAfter              Operator = "after"
	OperatorSemVerEqual        Operator = "semVerEqual"
	OperatorSemVerLessThan     Operator = "semVerLessThan"
	OperatorSemVerGreaterThan  Operator = "semVerGreaterThan"
)

type opFn (func(interface{}, interface{}) bool)

type Operator string

var OpsList = []Operator{
	OperatorIn,
	OperatorEndsWith,
	OperatorStartsWith,
	OperatorMatches,
	OperatorContains,
	OperatorLessThan,
	OperatorLessThanOrEqual,
	OperatorGreaterThan,
	OperatorGreaterThanOrEqual,
	OperatorBefore,
	OperatorAfter,
	OperatorSemVerEqual,
	OperatorSemVerLessThan,
	OperatorSemVerGreaterThan,
}

var versionNumericComponentsRegex, _ = regexp.Compile("^[0-9.]*")

func (op Operator) Name() string {
	return string(op)
}

var allOps = map[Operator]opFn{
	OperatorIn:                 operatorInFn,
	OperatorEndsWith:           operatorEndsWithFn,
	OperatorStartsWith:         operatorStartsWithFn,
	OperatorMatches:            operatorMatchesFn,
	OperatorContains:           operatorContainsFn,
	OperatorLessThan:           operatorLessThanFn,
	OperatorLessThanOrEqual:    operatorLessThanOrEqualFn,
	OperatorGreaterThan:        operatorGreaterThanFn,
	OperatorGreaterThanOrEqual: operatorGreaterThanOrEqualFn,
	OperatorBefore:             operatorBeforeFn,
	OperatorAfter:              operatorAfterFn,
	OperatorSemVerEqual:        operatorSemVerEqualFn,
	OperatorSemVerLessThan:     operatorSemVerLessThanFn,
	OperatorSemVerGreaterThan:  operatorSemVerGreaterThanFn,
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

func parseSemVer(value interface{}) (semver.Version, bool) {
	if str, ok := value.(string); ok {
		for tries := 0; tries < 3; tries++ {
			if sv, err := semver.Parse(str); err == nil {
				return sv, true
			}
			if tries < 2 {
				matched := versionNumericComponentsRegex.FindString(str)
				if matched == "" {
					break
				}
				str = matched + ".0" + str[len(matched):]
			}
		}
	}
	return semver.Version{}, false
}

func semVerOperator(uValue interface{}, cValue interface{}, fn func(semver.Version, semver.Version) bool) bool {
	u, uOk := parseSemVer(uValue)
	c, cOk := parseSemVer(cValue)
	return uOk && cOk && fn(u, c)
}

func operatorSemVerEqualFn(uValue interface{}, cValue interface{}) bool {
	return semVerOperator(uValue, cValue, semver.Version.Equals)
}

func operatorSemVerLessThanFn(uValue interface{}, cValue interface{}) bool {
	return semVerOperator(uValue, cValue, semver.Version.LT)
}

func operatorSemVerGreaterThanFn(uValue interface{}, cValue interface{}) bool {
	return semVerOperator(uValue, cValue, semver.Version.GT)
}

func operatorNoneFn(uValue interface{}, cValue interface{}) bool {
	return false
}

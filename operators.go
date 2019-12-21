package ldclient

import (
	"regexp"
	"strings"

	"github.com/blang/semver"
)

// List of available operators
const (
	operatorIn                 operator = "in"
	operatorEndsWith           operator = "endsWith"
	operatorStartsWith         operator = "startsWith"
	operatorMatches            operator = "matches"
	operatorContains           operator = "contains"
	operatorLessThan           operator = "lessThan"
	operatorLessThanOrEqual    operator = "lessThanOrEqual"
	operatorGreaterThan        operator = "greaterThan"
	operatorGreaterThanOrEqual operator = "greaterThanOrEqual"
	operatorBefore             operator = "before"
	operatorAfter              operator = "after"
	operatorSegmentMatch       operator = "segmentMatch"
	operatorSemVerEqual        operator = "semVerEqual"
	operatorSemVerLessThan     operator = "semVerLessThan"
	operatorSemVerGreaterThan  operator = "semVerGreaterThan"
)

type opFn (func(interface{}, interface{}) bool)

// operator describes an operator for a clause.
type operator string

var versionNumericComponentsRegex = regexp.MustCompile(`^\d+(\.\d+)?(\.\d+)?`)

// Name returns the string name for an operator
func (op operator) Name() string {
	return string(op)
}

var allOps = map[operator]opFn{
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
	operatorSemVerEqual:        operatorSemVerEqualFn,
	operatorSemVerLessThan:     operatorSemVerLessThanFn,
	operatorSemVerGreaterThan:  operatorSemVerGreaterThanFn,
}

// Turn this into a static map
func operatorFn(operator operator) opFn {
	if op, ok := allOps[operator]; ok {
		return op
	}
	return operatorNoneFn
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
		}
		return false
	})
}

func operatorContainsFn(uValue interface{}, cValue interface{}) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool { return strings.Contains(u, c) })
}

func numericOperator(uValue interface{}, cValue interface{}, fn func(float64, float64) bool) bool {
	uFloat64 := parseFloat64(uValue)
	if uFloat64 != nil {
		cFloat64 := parseFloat64(cValue)
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

func parseSemVer(value interface{}) (semver.Version, bool) {
	if versionStr, ok := value.(string); ok {
		if sv, err := semver.Parse(versionStr); err == nil {
			return sv, true
		}
		// Failed to parse as-is; see if we can fix it by adding zeroes
		matchParts := versionNumericComponentsRegex.FindStringSubmatch(versionStr)
		if matchParts != nil {
			transformedVersionStr := matchParts[0]
			for i := 1; i < len(matchParts); i++ {
				if matchParts[i] == "" {
					transformedVersionStr += ".0"
				}
			}
			transformedVersionStr += versionStr[len(matchParts[0]):]
			if sv, err := semver.Parse(transformedVersionStr); err == nil {
				return sv, true
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

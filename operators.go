package ldclient

import (
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// List of available operators
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
	OperatorSegmentMatch       Operator = "segmentMatch"
	OperatorSemVerEqual        Operator = "semVerEqual"
	OperatorSemVerLessThan     Operator = "semVerLessThan"
	OperatorSemVerGreaterThan  Operator = "semVerGreaterThan"
)

type opFn (func(ldvalue.Value, ldvalue.Value) bool)

// Operator describes an operator for a clause.
//
// Deprecated: this type is for internal use and will be moved to another package in a future version.
type Operator string

// OpsList is the list of available operators
//
// Deprecated: this variable is for internal use and will be removed in a future version.
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
	OperatorSegmentMatch,
	OperatorSemVerEqual,
	OperatorSemVerLessThan,
	OperatorSemVerGreaterThan,
}

var versionNumericComponentsRegex = regexp.MustCompile(`^\d+(\.\d+)?(\.\d+)?`)

// Name returns the string name for an operator
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
	}
	return operatorNoneFn
}

func operatorInFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return uValue.Equal(cValue)
}

func stringOperator(uValue ldvalue.Value, cValue ldvalue.Value, fn func(string, string) bool) bool {
	if uValue.Type() == ldvalue.StringType && cValue.Type() == ldvalue.StringType {
		return fn(uValue.StringValue(), cValue.StringValue())
	}
	return false
}

func operatorStartsWithFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool { return strings.HasPrefix(u, c) })
}

func operatorEndsWithFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool { return strings.HasSuffix(u, c) })
}

func operatorMatchesFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool {
		if matched, err := regexp.MatchString(c, u); err == nil {
			return matched
		}
		return false
	})
}

func operatorContainsFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return stringOperator(uValue, cValue, func(u string, c string) bool { return strings.Contains(u, c) })
}

func numericOperator(uValue ldvalue.Value, cValue ldvalue.Value, fn func(float64, float64) bool) bool {
	if uValue.IsNumber() && cValue.IsNumber() {
		return fn(uValue.Float64Value(), cValue.Float64Value())
	}
	return false
}

func operatorLessThanFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u < c })
}

func operatorLessThanOrEqualFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u <= c })
}

func operatorGreaterThanFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u > c })
}

func operatorGreaterThanOrEqualFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return numericOperator(uValue, cValue, func(u float64, c float64) bool { return u >= c })
}

func operatorBeforeFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	if u, ok := parseDateTime(uValue); ok {
		if c, ok := parseDateTime(cValue); ok {
			return u.Before(c)
		}
	}
	return false
}

func operatorAfterFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	if u, ok := parseDateTime(uValue); ok {
		if c, ok := parseDateTime(cValue); ok {
			return u.After(c)
		}
	}
	return false
}

func parseDateTime(value ldvalue.Value) (time.Time, bool) {
	switch value.Type() {
	case ldvalue.StringType:
		t, err := time.Parse(time.RFC3339Nano, value.StringValue())
		if err == nil {
			return t.UTC(), true
		}
	case ldvalue.NumberType:
		return unixMillisToUtcTime(value.Float64Value()), true
	}
	return time.Time{}, false
}

func parseSemVer(value ldvalue.Value) (semver.Version, bool) {
	if value.Type() == ldvalue.StringType {
		versionStr := value.StringValue()
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

func semVerOperator(uValue ldvalue.Value, cValue ldvalue.Value, fn func(semver.Version, semver.Version) bool) bool {
	u, uOk := parseSemVer(uValue)
	c, cOk := parseSemVer(cValue)
	return uOk && cOk && fn(u, c)
}

func operatorSemVerEqualFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return semVerOperator(uValue, cValue, semver.Version.Equals)
}

func operatorSemVerLessThanFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return semVerOperator(uValue, cValue, semver.Version.LT)
}

func operatorSemVerGreaterThanFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return semVerOperator(uValue, cValue, semver.Version.GT)
}

func operatorNoneFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return false
}

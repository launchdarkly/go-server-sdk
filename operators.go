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

type opFn (func(ldvalue.Value, ldvalue.Value) bool)

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

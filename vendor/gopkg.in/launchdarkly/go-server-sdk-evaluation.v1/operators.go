package evaluation

import (
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// List of available operators
const (
	// OperatorIn matches a user value and clause value if the two values are equal (including their type).
	OperatorIn Operator = "in"
	// OperatorEndsWith matches a user value and clause value if they are both strings and the former ends with
	// the latter.
	OperatorEndsWith Operator = "endsWith"
	// OperatorStartsWith matches a user value and clause value if they are both strings and the former starts
	// with the latter.
	OperatorStartsWith Operator = "startsWith"
	// OperatorMatches matches a user value and clause value if they are both strings and the latter is a valid
	// regular expression that matches the former.
	OperatorMatches Operator = "matches"
	// OperatorContains matches a user value and clause value if they are both strings and the former contains
	// the latter.
	OperatorContains Operator = "contains"
	// OperatorLessThan matches a user value and clause value if they are both numbers and the former < the
	// latter.
	OperatorLessThan Operator = "lessThan"
	// OperatorLessThanOrEqual matches a user value and clause value if they are both numbers and the former
	// <= the latter.
	OperatorLessThanOrEqual Operator = "lessThanOrEqual"
	// OperatorGreaterThan matches a user value and clause value if they are both numbers and the former > the
	// latter.
	OperatorGreaterThan Operator = "greaterThan"
	// OperatorGreaterThanOrEqual matches a user value and clause value if they are both numbers and the former
	// >= the latter.
	OperatorGreaterThanOrEqual Operator = "greaterThanOrEqual"
	// OperatorBefore matches a user value and clause value if they are both timestamps and the former < the
	// latter.
	//
	// A valid timestamp is either a string in RFC3339/ISO8601 format, or a number which is treated as Unix
	// milliseconds.
	OperatorBefore Operator = "before"
	// OperatorAfter matches a user value and clause value if they are both timestamps and the former > the
	// latter.
	//
	// A valid timestamp is either a string in RFC3339/ISO8601 format, or a number which is treated as Unix
	// milliseconds.
	OperatorAfter Operator = "after"
	// OperatorSegmentMatch matches a user if the user is included in the user segment whose key is the clause
	// value.
	OperatorSegmentMatch Operator = "segmentMatch"
	// OperatorSemVerEqual matches a user value and clause value if they are both semantic versions and they
	// are equal.
	//
	// A semantic version is a string that either follows the Semantic Versions 2.0 spec, or is an abbreviated
	// version consisting of digits and optional periods in the form "m" (equivalent to m.0.0) or "m.n"
	// (equivalent to m.n.0).
	OperatorSemVerEqual Operator = "semVerEqual"
	// OperatorSemVerLessThan matches a user value and clause value if they are both semantic versions and the
	// former < the latter.
	//
	// A semantic version is a string that either follows the Semantic Versions 2.0 spec, or is an abbreviated
	// version consisting of digits and optional periods in the form "m" (equivalent to m.0.0) or "m.n"
	// (equivalent to m.n.0).
	OperatorSemVerLessThan Operator = "semVerLessThan"
	// OperatorSemVerGreaterThan matches a user value and clause value if they are both semantic versions and
	// the former > the latter.
	//
	// A semantic version is a string that either follows the Semantic Versions 2.0 spec, or is an abbreviated
	// version consisting of digits and optional periods in the form "m" (equivalent to m.0.0) or "m.n"
	// (equivalent to m.n.0).
	OperatorSemVerGreaterThan Operator = "semVerGreaterThan"
)

type opFn (func(ldvalue.Value, ldvalue.Value) bool)

// Operator describes an operator for a clause.
type Operator string

var versionNumericComponentsRegex = regexp.MustCompile(`^\d+(\.\d+)?(\.\d+)?`)

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
	return stringOperator(uValue, cValue, strings.HasPrefix)
}

func operatorEndsWithFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return stringOperator(uValue, cValue, strings.HasSuffix)
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
	return stringOperator(uValue, cValue, strings.Contains)
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

func dateOperator(uValue ldvalue.Value, cValue ldvalue.Value, fn func(time.Time, time.Time) bool) bool {
	if uTime, ok := parseDateTime(uValue); ok {
		if cTime, ok := parseDateTime(cValue); ok {
			return fn(uTime, cTime)
		}
	}
	return false
}

func operatorBeforeFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return dateOperator(uValue, cValue, time.Time.Before)
}

func operatorAfterFn(uValue ldvalue.Value, cValue ldvalue.Value) bool {
	return dateOperator(uValue, cValue, time.Time.After)
}

func semVerOperator(uValue ldvalue.Value, cValue ldvalue.Value, fn func(semver.Version, semver.Version) bool) bool {
	if u, ok := parseSemVer(uValue); ok {
		if c, ok := parseSemVer(cValue); ok {
			return fn(u, c)
		}
	}
	return false
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

func unixMillisToUtcTime(unixMillis float64) time.Time {
	return time.Unix(0, int64(unixMillis)*int64(time.Millisecond)).UTC()
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

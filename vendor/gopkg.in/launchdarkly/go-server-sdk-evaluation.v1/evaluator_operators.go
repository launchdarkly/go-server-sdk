package evaluation

import (
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

type opFn (func(ldvalue.Value, ldvalue.Value) bool)

var versionNumericComponentsRegex = regexp.MustCompile(`^\d+(\.\d+)?(\.\d+)?`)

var allOps = map[ldmodel.Operator]opFn{
	ldmodel.OperatorIn:                 operatorInFn,
	ldmodel.OperatorEndsWith:           operatorEndsWithFn,
	ldmodel.OperatorStartsWith:         operatorStartsWithFn,
	ldmodel.OperatorMatches:            operatorMatchesFn,
	ldmodel.OperatorContains:           operatorContainsFn,
	ldmodel.OperatorLessThan:           operatorLessThanFn,
	ldmodel.OperatorLessThanOrEqual:    operatorLessThanOrEqualFn,
	ldmodel.OperatorGreaterThan:        operatorGreaterThanFn,
	ldmodel.OperatorGreaterThanOrEqual: operatorGreaterThanOrEqualFn,
	ldmodel.OperatorBefore:             operatorBeforeFn,
	ldmodel.OperatorAfter:              operatorAfterFn,
	ldmodel.OperatorSemVerEqual:        operatorSemVerEqualFn,
	ldmodel.OperatorSemVerLessThan:     operatorSemVerLessThanFn,
	ldmodel.OperatorSemVerGreaterThan:  operatorSemVerGreaterThanFn,
}

func operatorFn(operator ldmodel.Operator) opFn {
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

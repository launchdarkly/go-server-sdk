package matchers

import (
	"fmt"
)

// Not negates the result of another Matcher.
//
//	matchers.Not(Equal(3)).Assert(t, 4)
//	// failure message will describe expectation as "not (equal to 3)"
func Not(matcher Matcher) Matcher {
	return New(
		func(value interface{}) bool {
			return !matcher.test(value)
		},
		func() string {
			return fmt.Sprintf("not (%s)", matcher.describeTest())
		},
		nil,
	)
}

// AllOf requires that the input value passes all of the specified Matchers. If it fails,
// the failure message describes all of the Matchers that failed.
func AllOf(matchers ...Matcher) Matcher {
	return New(
		func(value interface{}) bool {
			for _, m := range matchers {
				if !m.test(value) {
					return false
				}
			}
			return true
		},
		func() string {
			return describeMatchers(matchers, " and ")
		},
		func(value interface{}) string {
			return describeFailures(matchers, value)
		},
	)
}

// AnyOf requires that the input value passes at least one of the specified Matchers. It will
// not execute any further matches after the first pass. If it fails all of them, the failure
// message describes all of the failure conditions.
func AnyOf(matchers ...Matcher) Matcher {
	return New(
		func(value interface{}) bool {
			for _, m := range matchers {
				if m.test(value) {
					return true
				}
			}
			return false
		},
		func() string {
			return describeMatchers(matchers, " and ")
		},
		func(value interface{}) string {
			return describeFailures(matchers, value)
		},
	)
}

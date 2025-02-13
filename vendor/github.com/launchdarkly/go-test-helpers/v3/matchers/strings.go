package matchers

import (
	"fmt"
	"strings"
)

// StringContains is a matcher for string values that tests for the presence of a substring,
// case-sensitively.
func StringContains(substring string) Matcher {
	return New(
		func(value interface{}) bool {
			return strings.Contains(value.(string), substring)
		},
		func() string {
			return fmt.Sprintf("contains %q", substring)
		},
		func(interface{}) string {
			return fmt.Sprintf("did not contain %q", substring)
		},
	).EnsureType("")
}

// StringHasPrefix is a matcher for string values that calls strings.HasPrefix.
func StringHasPrefix(prefix string) Matcher {
	return New(
		func(value interface{}) bool {
			return strings.HasPrefix(value.(string), prefix)
		},
		func() string {
			return fmt.Sprintf("starts with %q", prefix)
		},
		func(interface{}) string {
			return fmt.Sprintf("did not start with %q", prefix)
		},
	).EnsureType("")
}

// StringHasSuffix is a matcher for string values that calls strings.HasSuffix.
func StringHasSuffix(suffix string) Matcher {
	return New(
		func(value interface{}) bool {
			return strings.HasSuffix(value.(string), suffix)
		},
		func() string {
			return fmt.Sprintf("ends with %q", suffix)
		},
		func(interface{}) string {
			return fmt.Sprintf("did not end with %q", suffix)
		},
	).EnsureType("")
}

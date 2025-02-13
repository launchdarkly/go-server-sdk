package jsonhelpers

import (
	"reflect"
	"strings"

	"github.com/stretchr/testify/assert"
)

// AssertEqual compares two JSON Value instances and returns true if they are deeply equal.
// If they are not equal, it outputs a test failure message describing the mismatch as
// specifically as possible.
//
// The two values may either be pre-parsed JValue instances, or if they are not, they are
// converted using the same rules as JValueOf.
func AssertEqual(t assert.TestingT, expected, actual any) bool {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	ev, av := JValueOf(expected), JValueOf(actual)
	if ev.err != nil {
		t.Errorf("invalid expected value (%s): %s", ev.err, ev)
		return false
	}
	if av.err != nil {
		t.Errorf("invalid actual value (%s): %s", av.err, av)
		return false
	}
	if reflect.DeepEqual(ev.parsed, av.parsed) {
		return true
	}
	diff := describeValueDifference(ev.parsed, av.parsed, nil)
	if len(diff) == 1 && diff[0].Path == nil {
		t.Errorf("expected JSON value: %s\nactual value: %s", expected, actual)
	} else {
		t.Errorf("incorrect JSON value: %s\n"+strings.Join(diff.Describe("expected", "actual"), "\n"), actual)
	}
	return false
}

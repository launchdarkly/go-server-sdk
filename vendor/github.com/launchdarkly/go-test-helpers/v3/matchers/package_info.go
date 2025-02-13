// Package matchers provides a flexible test assertion API similar to Java's Hamcrest. Matchers are
// constructed separately from the values being tested, and can then be applied to any value, or
// negated, or combined in various ways.
//
// This implementation is for Go 1.17 so it does not yet have generics. Instead, all matchers take
// values of type interface{} and must explicitly cast the type if needed. The simplest way to
// provide type safety is to use Matcher.EnsureType().
//
// Examples of syntax:
//
//	import m "github.com/launchdarkly/go-test-helpers/matchers"
//
//	func TestSomething(t *T) {
//	    eventData := []string{
//	        `{"kind": "feature", "value": true}`,
//	        `{"key": "x", "kind": "custom"}`,
//	    }
//	    m.For(t, "event data").Assert(eventData, m.ItemsInAnyOrder(
//	        m.JSONStrEqual(`{"kind": "custom", "key": "x"}`),
//	        m.JSONStrEqual(`{"kind": "feature", "value": true}`),
//	    ))
//	    m.For(t, "first event").Assert(eventData[0],
//	        m.JSONProperty("kind").Should(m.Not(m.Equal("summary"))))
//	}
package matchers

package matchers

// TestingT is an interface for any test scope type that has an Errorf method for reporting
// failures, and a FailNow method for stopping the test immediately. This is compatible with
// Go's testing.T, and with assert.TestingT and require.TestingT. See Test and For.
type TestingT interface {
	Errorf(format string, args ...interface{})
	FailNow()
}

// Go's testing.T and other compatible packages provide an additional method, Helper(),
// which indicates that whatever function called it should not be included in stacktraces.
type helperT interface {
	Helper()
}

// AssertionScope is a context for executing assertions.
type AssertionScope struct {
	t      TestingT
	prefix string
}

// In is for use with any test framework that has a test scope type with the same basic methods
// as Go's testing.T (as defined by the TestingT interface). Any calls to Assert or Require on
// the returned AssertionScope will update the state of t.
//
//	func TestSomething(t *testing.T) {
//	    matchers.In(t).Assert(x, matchers.Equal(2))
//	}
//
// See also For.
func In(t TestingT) AssertionScope {
	return AssertionScope{t: t}
}

// For is the same as In, but adds a descriptive name in front of whatever assertions are done.
// In this example, a failure would be logged as "score: does not equal 2" rather than only
// "does not equal 2".
//
//	func TestSomething(t *testing.T) {
//	    matchers.For(t, "score").Assert(x, matchers.Equal(2))
//	}
func For(t TestingT, name string) AssertionScope {
	return AssertionScope{t: t, prefix: name + ": "}
}

// For returns a new AssertionScope that has an additional name prefix. In this example,
// a failure would be logged as "final: score: does not equal 2" rather than only
// "does not equal 2".
//
//	func TestSomething(t *testing.T) {
//	    matchers.In(t).For("final").For("score").Assert(x, matchers.Equal(2))
//	}
func (a AssertionScope) For(name string) AssertionScope {
	return AssertionScope{t: a.t, prefix: a.prefix + name + ": "}
}

// Assert is for use with any test framework that has a test scope type with the same Errorf
// method as Go's testing.T. It tests a value against a matcher and, on failure, calls the test
// scope's Errorf method. This logs a failure but does not stop the test.
func (a AssertionScope) Assert(value interface{}, matcher Matcher) bool {
	if pass, desc := matcher.Test(value); !pass {
		if h, ok := a.t.(helperT); ok {
			h.Helper()
		}
		a.fail(desc)
		return false
	}
	return true
}

// Require is for use with any test framework that has a test scope type with the same Errorf
// and FailNow methods as Go's testing.T. It tests a value against a matcher and, on failure, calls
// the test scope's Errorf method and then FailNow. This logs a failure and immediately terminates
// the test.
func (a AssertionScope) Require(value interface{}, matcher Matcher) bool {
	if pass, desc := matcher.Test(value); !pass {
		if h, ok := a.t.(helperT); ok {
			h.Helper()
		}
		a.fail(desc)
		a.t.FailNow()
		return false // does not return since FailNow() will force an early exit
	}
	return true
}

func (a AssertionScope) fail(desc string) {
	if h, ok := a.t.(helperT); ok {
		h.Helper()
	}
	a.t.Errorf("%s%s", a.prefix, desc)
}

package testbox

import "testing"

type realTestingT struct {
	t *testing.T
}

// RealTest provides an implementation of TestingT for running test logic in a regular test context.
//
// See TestingT for details.
func RealTest(t *testing.T) TestingT {
	return realTestingT{t}
}

func (r realTestingT) Errorf(format string, args ...interface{}) {
	r.t.Errorf(format, args...) // COVERAGE: can't do this in test_sandbox_test; it'll cause a real failure
}

func (r realTestingT) Run(name string, action func(TestingT)) {
	r.t.Run(name, func(tt *testing.T) { action(realTestingT{tt}) })
}

func (r realTestingT) FailNow() {
	r.t.FailNow() // COVERAGE: can't do this in test_sandbox_test; it'll cause a real failure
}

func (r realTestingT) Failed() bool {
	return r.t.Failed()
}

func (r realTestingT) Skip(args ...interface{}) {
	r.t.Skip(args...)
}

func (r realTestingT) SkipNow() {
	r.t.SkipNow()
}

package testbox

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/stretchr/testify/assert"
)

// SandboxResult describes the aggregate test state produced by calling SandboxTest.
type SandboxResult struct {
	// True if any failures were reported during SandboxTest.
	Failed bool

	// True if the test run with SandboxTest called Skip or SkipNow. This is only true if
	// the top-level TestingT was skipped, not any subtests.
	Skipped bool

	// All failures logged during SandboxTest, including subtests.
	Failures []LogItem

	// All tests that were skipped during SandboxTest, including subtests.
	Skips []LogItem
}

type testState struct {
	failed   bool
	skipped  bool
	failures []LogItem
	skips    []LogItem
}

// TestPath identifies the level of test that failed or skipped. SandboxResult.Failures and
// SandboxResult.Skips use this type to distinguish between the top-level test that was run with
// SandboxTest and subtests that were run within that test with TestingT.Run(). A nil value means the
// top-level test; a single string element is the name of a subtest run from the top level with
// TestingT.Run(); nested subtests add an element for each level.
type TestPath []string

// LogItem describes either a failed assertion or a skip that happened during SandboxTest.
type LogItem struct {
	// Path identifies the level of test that failed or was skipped.
	Path TestPath

	// Message is the failure message or skip message, if any. It is the result of calling fmt.Sprintf
	// or Sprintln on the arguments that were passed to TestingT.Errorf or TestingT.Skip. If a test
	// failed without specifying a message, this is "".
	Message string
}

type mockTestingT struct {
	testState
	path TestPath
	lock sync.Mutex
}

// SandboxTest runs a test function against a TestingT instance that applies only to the scope of
// that test. If the function makes a failed assertion, marks the test as skipped, or forces an early
// exit with FailNow or SkipNow, this is reflected in the SandboxResult but does not affect the state
// of the regular test framework (assuming that this code is even executing within a Go test; it does
// not have to be).
//
// The reason this uses a callback function parameter, rather than simply having the SandboxResult
// implement TestingT itself, is that the function must be run on a separate goroutine so that
// the sandbox can intercept any early exits from FailNow or SkipNow.
//
// SandboxTest does not recover from panics.
//
// See TestingT for more details.
func SandboxTest(action func(TestingT)) SandboxResult {
	sub := new(mockTestingT)
	sub.runSafely(action)
	state := sub.getState()
	return SandboxResult{
		Failed:   state.failed,
		Skipped:  state.skipped,
		Failures: state.failures,
		Skips:    state.skips,
	}
}

func (m *mockTestingT) Errorf(format string, args ...interface{}) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.failed = true
	m.failures = append(m.failures, LogItem{Path: m.path, Message: fmt.Sprintf(format, args...)})
}

func (m *mockTestingT) Run(name string, action func(TestingT)) {
	sub := &mockTestingT{path: append(m.path, name)}
	sub.runSafely(action)
	subState := sub.getState()

	m.lock.Lock()
	defer m.lock.Unlock()
	m.failed = m.failed || subState.failed
	m.failures = append(m.failures, subState.failures...)
	m.skips = append(m.skips, subState.skips...)
}

func (m *mockTestingT) FailNow() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.testState.failed = true
	runtime.Goexit()
}

func (m *mockTestingT) Failed() bool {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.failed
}

func (m *mockTestingT) Skip(args ...interface{}) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.skipped = true
	m.skips = append(m.skips, LogItem{Path: m.path, Message: strings.TrimSuffix(fmt.Sprintln(args...), "\n")})
	runtime.Goexit()
}

func (m *mockTestingT) SkipNow() {
	m.Skip()
}

func (m *mockTestingT) getState() testState {
	m.lock.Lock()
	defer m.lock.Unlock()
	ret := testState{failed: m.failed, skipped: m.skipped}
	if len(m.failures) > 0 {
		ret.failures = make([]LogItem, len(m.failures))
		copy(ret.failures, m.failures)
	}
	if len(m.skips) > 0 {
		ret.skips = make([]LogItem, len(m.skips))
		copy(ret.skips, m.skips)
	}
	return ret
}

func (m *mockTestingT) runSafely(action func(TestingT)) {
	exited := make(chan struct{}, 1)
	go func() {
		defer func() {
			close(exited)
		}()
		action(m)
	}()
	<-exited
}

// ShouldFail is a shortcut for running some action against a testbox.TestingT and
// asserting that it failed.
func ShouldFail(t assert.TestingT, action func(TestingT)) bool {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	shouldGetHere := make(chan struct{}, 1)
	result := SandboxTest(func(t1 TestingT) {
		action(t1)
		shouldGetHere <- struct{}{}
	})
	if !result.Failed {
		t.Errorf("expected test to fail, but it passed")
		return false
	}
	if len(shouldGetHere) == 0 {
		t.Errorf("test failed as expected, but it also terminated early and should not have")
		return false
	}
	return true
}

// ShouldFailAndExitEarly is the same as ShouldFail, except that it also asserts that
// the test was terminated early with FailNow.
func ShouldFailAndExitEarly(t assert.TestingT, action func(TestingT)) bool {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	shouldNotGetHere := make(chan struct{}, 1)
	result := SandboxTest(func(t1 TestingT) {
		action(t1)
		shouldNotGetHere <- struct{}{}
	})
	if !result.Failed {
		t.Errorf("expected test to fail, but it passed")
		return false
	}
	if len(shouldNotGetHere) != 0 {
		t.Errorf("test failed as expected, but it should have also terminated early and did not")
		return false
	}
	return true
}

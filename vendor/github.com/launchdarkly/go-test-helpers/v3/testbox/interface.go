package testbox

import "github.com/stretchr/testify/require"

// TestingT is a subset of the testing.T interface that allows tests to run in either a real test
// context, or a mock test scope that is decoupled from the regular testing framework (SandboxTest).
//
// This may be useful in a scenario where you have a contract test that verifies the behavior of some
// unknown interface implementation. In order to verify that the contract test is reliable, you could
// create implementations that either adhere to the contract or deliberately break it, run the contract
// test against those, and verify that the test fails if and only if it should fail.
//
// The reason this cannot be done with the Go testing package alone is that the standard testing.T type
// cannot be created from within test code; instances are always passed in from the test framework.
// Therefore, the contract test would have to be run against the actual *testing.T instance that
// belongs to the test-of-the-test, and if it failed in a situation when we actually wanted it to
// to fail, that would be incorrectly reported as a failure of the test-of-the-test.
//
// To work around this limitation of the testing package, this package provides a TestingT interface
// that has two implementations: real and mock. Test logic can then be written against this TestingT,
// rather than *testing.T.
//
//	func RunContractTests(t *testing.T, impl InterfaceUnderTest) {
//	    runContractTests(testbox.RealTest(t))
//	}
//
//	func runContractTests(abstractT testbox.TestingT, impl InterfaceUnderTest) {
//	    assert.True(abstractT, impl.SomeConditionThatShouldBeTrueForTheseInputs(someParams))
//	    abstractT.Run("subtest", func(abstractSubT helpers.TestingT) { ... }
//	}
//
//	func TestContractTestFailureCondition(t *testing.T) {
//	    impl := createDeliberatelyBrokenImplementation()
//	    result := testbox.SandboxTest(func(abstractT testbox.TestingT) {
//	        runContractTests(abstractT, impl) })
//	    assert.True(t, result.Failed // we expect it to fail
//	    assert.Len(t, result.Failures, 1)
//	}
//
// TestingT includes the same subsets of testing.T methods that are defined in the TestingT interfaces
// of github.com/stretchr/testify/assert and github.com/stretchr/testify/require, so all assertions in
// those packages will work. It also provides Run, Skip, and SkipNow. It does not support Parallel.
type TestingT interface {
	require.TestingT
	// Run runs a subtest with a new TestingT that applies only to the scope of the subtest. It is
	// equivalent to the same method in testing.T, except the subtest takes a parameter of type TestingT
	// instead of *testing.T.
	//
	// If the subtest fails, the parent test also fails, but FailNow and SkipNow on the subtest do not
	// cause the parent test to exit early.
	Run(name string, action func(TestingT))

	// Failed tells whether whether any assertions in the test have failed so far. It is equivalent to
	// the same method in testing.T.
	Failed() bool

	// Skip marks the test as skipped and exits early, logging a message. It is equivalent to the same
	// method in testing.T.
	Skip(args ...interface{})

	// SkipNow marks the test as skipped and exits early. It is equivalent to the same method in testing.T.
	SkipNow()
}

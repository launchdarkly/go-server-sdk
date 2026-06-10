package testhelpers

import (
	"os"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// Fallible is a general interface for anything with a Failed method. This is used by test helpers to
// generalize between *testing.T, assert.T, etc. when all that we care about is detecting test failure.
type Fallible interface {
	Failed() bool
}

// WithMockLoggingContext creates a ClientContext that writes to a MockLogger, executes the specified
// action, and then dumps the captured output to the console only if there's been a test failure.
func WithMockLoggingContext(t Fallible, action func(subsystems.ClientContext)) {
	mockLog := ldlogtest.NewMockLog()
	context := sharedtest.NewTestContext("", nil,
		&subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
	defer func() {
		if t.Failed() {
			mockLog.Dump(os.Stdout)
		}
		// There's already a similar DumpLogIfTestFailed defined in the ldlogtest package, but it requires
		// specifically a *testing.T.
	}()
	action(context)
}

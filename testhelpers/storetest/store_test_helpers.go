package storetest

import (
	"os"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

type testCanFail interface {
	Failed() bool
}

// Creates a ClientContext that writes to a MockLogger; at the end of the action's scope, the captured
// output is dumped to the console only if there's been a test failure. The test parameter is declared
// as type testCanFail instead of *testing.T to allow us to use other test interface types (otherwise we
// could just use the existing MockLog.DumpIfTestFailed method, which takes a *testing.T).
func withMockLoggingContext(t testCanFail, action func(interfaces.ClientContext)) {
	mockLog := ldlogtest.NewMockLog()
	context := sharedtest.NewTestContext("", sharedtest.TestHTTPConfig(),
		sharedtest.TestLoggingConfigWithLoggers(mockLog.Loggers),
	)
	defer func() {
		if t.Failed() {
			mockLog.Dump(os.Stdout)
		}
	}()
	action(context)
}

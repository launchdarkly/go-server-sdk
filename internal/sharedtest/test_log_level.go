//nolint:gochecknoglobals,golint,stylecheck
package sharedtest

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

var testLogLevel = ldlog.None

// NewTestLoggers returns a standardized logger instance used by unit tests. If you want to temporarily
// enable log output for tests, change testLogLevel to for instance ldlog.Debug. Note that "go test"
// normally suppresses output anyway unless a test fails.
func NewTestLoggers() ldlog.Loggers {
	ret := ldlog.NewDefaultLoggers()
	ret.SetMinLevel(testLogLevel)
	return ret
}

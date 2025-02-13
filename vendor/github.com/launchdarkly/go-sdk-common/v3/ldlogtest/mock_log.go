package ldlogtest

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
)

// MockLogItem represents a log message captured by MockLog.
type MockLogItem struct {
	Level   ldlog.LogLevel
	Message string
}

// MockLog provides the ability to capture log output.
//
// It contains an [ldlog.Loggers] instance which can be used like any other Loggers, but all of the output is
// captured by the enclosing MockLog and can be retrieved with the MockLog methods.
//
//	mockLog := ldlogtest.NewMockLog()
//	mockLog.Loggers.Warn("message")
//	mockLog.Loggers.Warnf("also: %t", true)
//	warnMessages := mockLog.GetOutput(ldlog.Warn) // returns {"message", "also: true"}
type MockLog struct {
	// Loggers is the ldlog.Loggers instance to be used for tests.
	Loggers ldlog.Loggers
	// Output is a map containing all of the lines logged for each level. The level prefix is removed from the text.
	output map[ldlog.LogLevel][]string
	// AllOutput is a list of all the log output for any level in order. The level prefix is removed from the text.
	allOutput []MockLogItem
	lock      sync.Mutex
}

// NewMockLog creates a log-capturing object.
func NewMockLog() *MockLog {
	ret := &MockLog{output: make(map[ldlog.LogLevel][]string)}
	for _, level := range []ldlog.LogLevel{ldlog.Debug, ldlog.Info, ldlog.Warn, ldlog.Error} {
		ret.Loggers.SetBaseLoggerForLevel(level, mockBaseLogger{owner: ret, level: level})
	}
	return ret
}

// GetOutput returns the captured output for a specific log level.
func (ml *MockLog) GetOutput(level ldlog.LogLevel) []string {
	ml.lock.Lock()
	defer ml.lock.Unlock()
	lines := ml.output[level]
	ret := make([]string, len(lines))
	copy(ret, lines)
	return ret
}

// GetAllOutput returns the captured output for all log levels.
func (ml *MockLog) GetAllOutput() []MockLogItem {
	ml.lock.Lock()
	defer ml.lock.Unlock()
	ret := make([]MockLogItem, len(ml.allOutput))
	copy(ret, ml.allOutput)
	return ret
}

// HasMessageMatch tests whether there is a log message of this level that matches this regex.
func (ml *MockLog) HasMessageMatch(level ldlog.LogLevel, pattern string) bool {
	_, found := ml.findMessageMatching(level, pattern)
	return found
}

// AssertMessageMatch asserts whether there is a log message of this level that matches this regex.
// This is equivalent to using [assert.True] or [assert.False] with [MockLog.HasMessageMatch], except that
// if the test fails, it includes the actual log messages in the failure message.
func (ml *MockLog) AssertMessageMatch(t *testing.T, shouldMatch bool, level ldlog.LogLevel, pattern string) {
	line, hasMatch := ml.findMessageMatching(level, pattern)
	if hasMatch != shouldMatch {
		if shouldMatch {
			assert.Fail(
				t,
				"log did not contain expected message",
				"level: %s, pattern: /%s/, messages: %v",
				level,
				pattern,
				ml.GetOutput(level),
			)
		} else {
			assert.Fail(
				t,
				"log contained unexpected message",
				"level: %s, message: [%s]",
				level,
				line,
			)
		}
	}
}

// Dump is a shortcut for writing all captured log lines to a Writer.
func (ml *MockLog) Dump(w io.Writer) {
	for _, line := range ml.GetAllOutput() {
		fmt.Fprintln(w, line.Level.Name()+": "+line.Message)
	}
}

// DumpIfTestFailed is a shortcut for writing all captured log lines to standard output only if
// t.Failed() is true.
//
// This is useful in tests where you normally don't want to see the log output, but you do want to see it
// if there was a failure. The simplest way to do this is to use defer:
//
//	func TestSomething(t *testing.T) {
//	    ml := ldlogtest.NewMockLog()
//	    defer ml.DumpIfTestFailed(t)
//	    // ... do some test things that may generate log output and/or cause a failure
//	}
func (ml *MockLog) DumpIfTestFailed(t *testing.T) {
	if t.Failed() { // COVERAGE: there's no way to test this in unit tests
		ml.Dump(os.Stdout)
	}
}

func (ml *MockLog) logLine(level ldlog.LogLevel, line string) {
	ml.lock.Lock()
	defer ml.lock.Unlock()
	message := strings.TrimPrefix(line, strings.ToUpper(level.String())+": ")
	ml.output[level] = append(ml.output[level], message)
	ml.allOutput = append(ml.allOutput, MockLogItem{level, message})
}

func (ml *MockLog) findMessageMatching(level ldlog.LogLevel, pattern string) (string, bool) {
	r := regexp.MustCompile(pattern)
	for _, line := range ml.GetOutput(level) {
		if r.MatchString(line) {
			return line, true
		}
	}
	return "", false
}

type mockBaseLogger struct {
	owner *MockLog
	level ldlog.LogLevel
}

func (l mockBaseLogger) Println(values ...interface{}) {
	l.owner.logLine(l.level, strings.TrimSuffix(fmt.Sprintln(values...), "\n"))
}

func (l mockBaseLogger) Printf(format string, values ...interface{}) {
	l.owner.logLine(l.level, fmt.Sprintf(format, values...))
}

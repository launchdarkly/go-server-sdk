package sharedtest

import (
	"fmt"
	"strings"
	"sync"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

// MockLogItem represents a log message captured by MockLoggers.
type MockLogItem struct {
	level   ldlog.LogLevel
	message string
}

// MockLoggers provides the ability to capture log output.
type MockLoggers struct {
	// Loggers is the ldlog.Loggers instance to be used for tests.
	Loggers ldlog.Loggers
	// Output is a map containing all of the lines logged for each level. The level prefix is removed from the text.
	Output map[ldlog.LogLevel][]string
	// AllOutput is a list of all the log output for any level in order. The level prefix is removed from the text.
	AllOutput []MockLogItem
	lock      sync.Mutex
}

// NullLoggers returns a Loggers instance that suppresses all output.
func NullLoggers() ldlog.Loggers {
	ret := ldlog.Loggers{}
	ret.SetMinLevel(ldlog.None)
	return ret
}

// NewMockLoggers creates a log-capturing object.
func NewMockLoggers() *MockLoggers {
	ret := &MockLoggers{Output: make(map[ldlog.LogLevel][]string)}
	for _, level := range []ldlog.LogLevel{ldlog.Debug, ldlog.Info, ldlog.Warn, ldlog.Error} {
		ret.Loggers.SetBaseLoggerForLevel(level, mockBaseLogger{owner: ret, level: level})
	}
	return ret
}

func (ml *MockLoggers) logLine(level ldlog.LogLevel, line string) {
	ml.lock.Lock()
	defer ml.lock.Unlock()
	message := strings.TrimPrefix(line, strings.ToUpper(level.String())+": ")
	ml.Output[level] = append(ml.Output[level], message)
	ml.AllOutput = append(ml.AllOutput, MockLogItem{level, message})
}

type mockBaseLogger struct {
	owner *MockLoggers
	level ldlog.LogLevel
}

func (l mockBaseLogger) Println(values ...interface{}) {
	l.owner.logLine(l.level, strings.TrimSuffix(fmt.Sprintln(values...), "\n"))
}

func (l mockBaseLogger) Printf(format string, values ...interface{}) {
	l.owner.logLine(l.level, fmt.Sprintf(format, values...))
}

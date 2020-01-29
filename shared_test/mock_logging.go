package shared_test

import (
	"fmt"
	"sync"

	"gopkg.in/launchdarkly/go-server-sdk.v5/ldlog"
)

// MockLoggers provides the ability to capture log output.
type MockLoggers struct {
	// Loggers is the ldlog.Loggers instance to be used for tests.
	Loggers ldlog.Loggers
	// Output is a map containing all of the lines logged for each level.
	Output map[ldlog.LogLevel][]string
	lock   sync.Mutex
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
	ml.Output[level] = append(ml.Output[level], line)
}

type mockBaseLogger struct {
	owner *MockLoggers
	level ldlog.LogLevel
}

func (l mockBaseLogger) Println(values ...interface{}) {
	l.owner.logLine(l.level, fmt.Sprintln(values...))
}

func (l mockBaseLogger) Printf(format string, values ...interface{}) {
	l.owner.logLine(l.level, fmt.Sprintf(format, values...))
}

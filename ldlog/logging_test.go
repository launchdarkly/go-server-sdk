package ldlog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type logSink struct {
	output []string
}

func (l *logSink) Println(values ...interface{}) {
	l.output = append(l.output, strings.TrimSpace(fmt.Sprintln(values...)))
}

func (l *logSink) Printf(format string, values ...interface{}) {
	l.output = append(l.output, fmt.Sprintf(format, values...))
}

func TestCanWriteToUnconfiguredLogger(t *testing.T) {
	l := Loggers{}
	l.Warn("test message, please ignore") // just testing that we don't get a nil pointer
}

func TestLevelIsInfoByDefault(t *testing.T) {
	ls := logSink{}
	l := Loggers{}
	l.SetBaseLogger(&ls)
	l.Debug("0")
	l.Debugf("%s!", "1")
	l.Info("2")
	l.Infof("%s!", "3")
	l.Warn("4")
	l.Warnf("%s!", "5")
	l.Error("6")
	l.Errorf("%s!", "7")
	assert.Equal(t, []string{"INFO: 2", "INFO: 3!", "WARN: 4", "WARN: 5!", "ERROR: 6", "ERROR: 7!"}, ls.output)
}

func TestCanSetLevel(t *testing.T) {
	ls := logSink{}
	l := Loggers{}
	l.SetBaseLogger(&ls)
	l.SetMinLevel(Error)
	l.Debug("0")
	l.Debugf("%s!", "1")
	l.Info("2")
	l.Infof("%s!", "3")
	l.Warn("4")
	l.Warnf("%s!", "5")
	l.Error("6")
	l.Errorf("%s!", "7")
	assert.Equal(t, []string{"ERROR: 6", "ERROR: 7!"}, ls.output)
}

func TestCanSetLoggerForSpecificLevel(t *testing.T) {
	lsMain := logSink{}
	lsWarn := logSink{}
	l := Loggers{}
	l.SetBaseLoggerForLevel(Warn, &lsWarn)
	l.SetBaseLogger(&lsMain)
	l.Info("a")
	l.Warn("b")
	assert.Equal(t, []string{"INFO: a"}, lsMain.output)
	assert.Equal(t, []string{"WARN: b"}, lsWarn.output)
}

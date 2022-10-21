package ldcomponents

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"

	"github.com/stretchr/testify/assert"
)

func TestLoggingConfigurationBuilder(t *testing.T) {
	basicConfig := subsystems.BasicClientContext{}

	t.Run("defaults", func(t *testing.T) {
		c := Logging().Build(basicConfig)
		assert.False(t, c.LogEvaluationErrors)
		assert.False(t, c.LogContextKeyInErrors)
	})

	t.Run("LogDataSourceOutageAsErrorAfter", func(t *testing.T) {
		c := Logging().LogDataSourceOutageAsErrorAfter(time.Hour).Build(basicConfig)
		assert.Equal(t, time.Hour, c.LogDataSourceOutageAsErrorAfter)
	})

	t.Run("LogEvaluationErrors", func(t *testing.T) {
		c := Logging().LogEvaluationErrors(true).Build(basicConfig)
		assert.True(t, c.LogEvaluationErrors)
	})

	t.Run("LogContextKeyInErrors", func(t *testing.T) {
		c := Logging().LogContextKeyInErrors(true).Build(basicConfig)
		assert.True(t, c.LogContextKeyInErrors)
	})

	t.Run("Loggers", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c := Logging().Loggers(mockLoggers.Loggers).Build(basicConfig)
		assert.Equal(t, mockLoggers.Loggers, c.Loggers)
	})

	t.Run("MinLevel", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c := Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Error).Build(basicConfig)
		c.Loggers.Info("suppress this message")
		c.Loggers.Error("log this message")
		assert.Len(t, mockLoggers.GetOutput(ldlog.Info), 0)
		assert.Equal(t, []string{"log this message"}, mockLoggers.GetOutput(ldlog.Error))
	})

	t.Run("NoLogging", func(t *testing.T) {
		c := NoLogging().Build(basicConfig)
		assert.Equal(t, ldlog.NewDisabledLoggers(), c.Loggers)
	})

	t.Run("nil safety", func(t *testing.T) {
		var b *LoggingConfigurationBuilder = nil
		b = b.LogContextKeyInErrors(true).LogDataSourceOutageAsErrorAfter(0).LogEvaluationErrors(true).
			Loggers(ldlog.NewDefaultLoggers()).MinLevel(ldlog.Debug)
		_ = b.Build(subsystems.BasicClientContext{})
	})
}

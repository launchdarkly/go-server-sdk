package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
)

func TestLoggingConfigurationBuilder(t *testing.T) {
	basicConfig := interfaces.BasicConfiguration{}

	t.Run("defaults", func(t *testing.T) {
		c := Logging().CreateLoggingConfiguration(basicConfig)
		assert.False(t, c.LogEvaluationErrors)
		assert.False(t, c.LogContextKeyInErrors)
	})

	t.Run("LogDataSourceOutageAsErrorAfter", func(t *testing.T) {
		c := Logging().LogDataSourceOutageAsErrorAfter(time.Hour).CreateLoggingConfiguration(basicConfig)
		assert.Equal(t, time.Hour, c.LogDataSourceOutageAsErrorAfter)
	})

	t.Run("LogEvaluationErrors", func(t *testing.T) {
		c := Logging().LogEvaluationErrors(true).CreateLoggingConfiguration(basicConfig)
		assert.True(t, c.LogEvaluationErrors)
	})

	t.Run("LogContextKeyInErrors", func(t *testing.T) {
		c := Logging().LogContextKeyInErrors(true).CreateLoggingConfiguration(basicConfig)
		assert.True(t, c.LogContextKeyInErrors)
	})

	t.Run("Loggers", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c := Logging().Loggers(mockLoggers.Loggers).CreateLoggingConfiguration(basicConfig)
		assert.Equal(t, mockLoggers.Loggers, c.Loggers)
	})

	t.Run("MinLevel", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c := Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Error).CreateLoggingConfiguration(basicConfig)
		c.Loggers.Info("suppress this message")
		c.Loggers.Error("log this message")
		assert.Len(t, mockLoggers.GetOutput(ldlog.Info), 0)
		assert.Equal(t, []string{"log this message"}, mockLoggers.GetOutput(ldlog.Error))
	})

	t.Run("NoLogging", func(t *testing.T) {
		c := NoLogging().CreateLoggingConfiguration(basicConfig)
		assert.Equal(t, ldlog.NewDisabledLoggers(), c.Loggers)
	})

	t.Run("nil safety", func(t *testing.T) {
		var b *LoggingConfigurationBuilder = nil
		b = b.LogContextKeyInErrors(true).LogDataSourceOutageAsErrorAfter(0).LogEvaluationErrors(true).
			Loggers(ldlog.NewDefaultLoggers()).MinLevel(ldlog.Debug)
		_ = b.CreateLoggingConfiguration(sharedtest.NewSimpleTestContext("").GetBasic())
	})
}

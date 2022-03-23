package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
)

func TestLoggingConfigurationBuilder(t *testing.T) {
	basicConfig := interfaces.BasicConfiguration{}

	t.Run("defaults", func(t *testing.T) {
		c, err := Logging().CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.False(t, c.LogEvaluationErrors)
		assert.False(t, c.LogContextKeyInErrors)
	})

	t.Run("LogDataSourceOutageAsErrorAfter", func(t *testing.T) {
		c, err := Logging().LogDataSourceOutageAsErrorAfter(time.Hour).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.Equal(t, time.Hour, c.LogDataSourceOutageAsErrorAfter)
	})

	t.Run("LogEvaluationErrors", func(t *testing.T) {
		c, err := Logging().LogEvaluationErrors(true).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.True(t, c.LogEvaluationErrors)
	})

	t.Run("LogContextKeyInErrors", func(t *testing.T) {
		c, err := Logging().LogContextKeyInErrors(true).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.True(t, c.LogContextKeyInErrors)
	})

	t.Run("Loggers", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c, err := Logging().Loggers(mockLoggers.Loggers).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.Equal(t, mockLoggers.Loggers, c.Loggers)
	})

	t.Run("MinLevel", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c, err := Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Error).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		c.Loggers.Info("suppress this message")
		c.Loggers.Error("log this message")
		assert.Len(t, mockLoggers.GetOutput(ldlog.Info), 0)
		assert.Equal(t, []string{"log this message"}, mockLoggers.GetOutput(ldlog.Error))
	})

	t.Run("NoLogging", func(t *testing.T) {
		c, err := NoLogging().CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.Equal(t, ldlog.NewDisabledLoggers(), c.Loggers)
	})
}

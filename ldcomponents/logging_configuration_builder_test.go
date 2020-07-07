package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

func TestLoggingConfigurationBuilder(t *testing.T) {
	basicConfig := interfaces.BasicConfiguration{}

	t.Run("defaults", func(t *testing.T) {
		c, err := Logging().CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.False(t, c.IsLogEvaluationErrors())
		assert.False(t, c.IsLogUserKeyInErrors())
	})

	t.Run("LogDataSourceOutageAsErrorAfter", func(t *testing.T) {
		c, err := Logging().LogDataSourceOutageAsErrorAfter(time.Hour).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.Equal(t, time.Hour, c.GetLogDataSourceOutageAsErrorAfter())
	})

	t.Run("LogEvaluationErrors", func(t *testing.T) {
		c, err := Logging().LogEvaluationErrors(true).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.True(t, c.IsLogEvaluationErrors())
	})

	t.Run("LogUserKeyInErrors", func(t *testing.T) {
		c, err := Logging().LogUserKeyInErrors(true).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.True(t, c.IsLogUserKeyInErrors())
	})

	t.Run("Loggers", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c, err := Logging().Loggers(mockLoggers.Loggers).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.Equal(t, mockLoggers.Loggers, c.GetLoggers())
	})

	t.Run("MinLevel", func(t *testing.T) {
		mockLoggers := ldlogtest.NewMockLog()
		c, err := Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Error).CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		c.GetLoggers().Info("suppress this message")
		c.GetLoggers().Error("log this message")
		assert.Len(t, mockLoggers.GetOutput(ldlog.Info), 0)
		assert.Equal(t, []string{"log this message"}, mockLoggers.GetOutput(ldlog.Error))
	})

	t.Run("NoLogging", func(t *testing.T) {
		c, err := NoLogging().CreateLoggingConfiguration(basicConfig)
		require.NoError(t, err)
		assert.Equal(t, ldlog.NewDisabledLoggers(), c.GetLoggers())
	})
}

package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func TestLoggingConfigurationBuilder(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		c := Logging().CreateLoggingConfiguration()
		assert.False(t, c.IsLogEvaluationErrors())
		assert.False(t, c.IsLogUserKeyInErrors())
	})

	t.Run("LogEvaluationErrors", func(t *testing.T) {
		c := Logging().LogEvaluationErrors(true).CreateLoggingConfiguration()
		assert.True(t, c.IsLogEvaluationErrors())
	})

	t.Run("LogUserKeyInErrors", func(t *testing.T) {
		c := Logging().LogUserKeyInErrors(true).CreateLoggingConfiguration()
		assert.True(t, c.IsLogUserKeyInErrors())
	})

	t.Run("Loggers", func(t *testing.T) {
		mockLoggers := sharedtest.NewMockLoggers()
		c := Logging().Loggers(mockLoggers.Loggers).CreateLoggingConfiguration()
		assert.Equal(t, mockLoggers.Loggers, c.GetLoggers())
	})

	t.Run("MinLevel", func(t *testing.T) {
		mockLoggers := sharedtest.NewMockLoggers()
		c := Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Error).CreateLoggingConfiguration()
		c.GetLoggers().Info("suppress this message")
		c.GetLoggers().Error("log this message")
		assert.Nil(t, mockLoggers.Output[ldlog.Info])
		assert.Equal(t, []string{"log this message"}, mockLoggers.Output[ldlog.Error])
	})

	t.Run("NoLogging", func(t *testing.T) {
		c := NoLogging().CreateLoggingConfiguration()
		assert.Equal(t, ldlog.NewDisabledLoggers(), c.GetLoggers())
	})
}

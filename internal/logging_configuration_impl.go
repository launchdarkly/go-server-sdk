package internal

import (
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

// LoggingConfigurationImpl is the internal implementation of LoggingConfiguration.
type LoggingConfigurationImpl struct {
	LogDataSourceOutageAsErrorAfter time.Duration
	LogEvaluationErrors             bool
	LogUserKeyInErrors              bool
	Loggers                         ldlog.Loggers
}

// NewLoggingConfigurationImpl creates the internal implementation of LoggingConfiguration.
func NewLoggingConfigurationImpl(loggers ldlog.Loggers) LoggingConfigurationImpl {
	return LoggingConfigurationImpl{Loggers: loggers}
}

//nolint:golint,stylecheck // no doc comment for standard method
func (c LoggingConfigurationImpl) GetLogDataSourceOutageAsErrorAfter() time.Duration {
	return c.LogDataSourceOutageAsErrorAfter
}

//nolint:golint,stylecheck // no doc comment for standard method
func (c LoggingConfigurationImpl) IsLogEvaluationErrors() bool {
	return c.LogEvaluationErrors
}

//nolint:golint,stylecheck // no doc comment for standard method
func (c LoggingConfigurationImpl) IsLogUserKeyInErrors() bool {
	return c.LogUserKeyInErrors
}

//nolint:golint,stylecheck // no doc comment for standard method
func (c LoggingConfigurationImpl) GetLoggers() ldlog.Loggers {
	return c.Loggers
}

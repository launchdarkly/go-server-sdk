package internal

import "gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"

// LoggingConfigurationImpl is the internal implementation of LoggingConfiguration.
type LoggingConfigurationImpl struct {
	LogEvaluationErrors bool
	LogUserKeyInErrors  bool
	Loggers             ldlog.Loggers
}

// NewLoggingConfigurationImpl creates the internal implementation of LoggingConfiguration.
func NewLoggingConfigurationImpl(loggers ldlog.Loggers) LoggingConfigurationImpl {
	return LoggingConfigurationImpl{Loggers: loggers}
}

func (c LoggingConfigurationImpl) IsLogEvaluationErrors() bool { //nolint:golint // no doc comment for standard method
	return c.LogEvaluationErrors
}

func (c LoggingConfigurationImpl) IsLogUserKeyInErrors() bool { //nolint:golint // no doc comment for standard method
	return c.LogUserKeyInErrors
}

func (c LoggingConfigurationImpl) GetLoggers() ldlog.Loggers { //nolint:golint // no doc comment for standard method
	return c.Loggers
}

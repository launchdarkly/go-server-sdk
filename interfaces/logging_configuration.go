package interfaces

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

// LoggingConfiguration encapsulates the SDK's general logging configuration.
//
// See LoggingConfigurationBuilder for more details on these properties.
type LoggingConfiguration interface {
	// GetLoggers returns the configured ldlog.Loggers instance.
	GetLoggers() ldlog.Loggers

	// IsLogEvaluationErrors returns true if evaluation errors should be logged.
	IsLogEvaluationErrors() bool

	// IsLogUserKeyInErrors returns true if user keys may be included in logging.
	IsLogUserKeyInErrors() bool
}

// LoggingConfigurationFactory is an interface for a factory that creates a LoggingConfiguration.
type LoggingConfigurationFactory interface {
	// CreateLoggingConfiguration is called internally by the SDK to obtain the configuration.
	CreateLoggingConfiguration() LoggingConfiguration
}

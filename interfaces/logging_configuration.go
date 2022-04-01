package interfaces

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
)

// LoggingConfiguration encapsulates the SDK's general logging configuration.
//
// See ldcomponents.LoggingConfigurationBuilder for more details on these properties.
type LoggingConfiguration struct {
	// Loggers is a configured ldlog.Loggers instance for general SDK logging.
	Loggers ldlog.Loggers

	// LogDataSourceOutageAsErrorAfter is the time threshold, if any, after which the SDK
	// will log a data source outage at Error level instead of Warn level. See
	// LoggingConfigurationBuilderLogDataSourceOutageAsErrorAfter().
	LogDataSourceOutageAsErrorAfter time.Duration

	// LogEvaluationErrors is true if evaluation errors should be logged.
	LogEvaluationErrors bool

	// LogContextKeyInErrors is true if context keys may be included in logging.
	LogContextKeyInErrors bool
}

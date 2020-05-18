package ldcomponents

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

// LoggingConfigurationBuilder contains methods for configuring the SDK's logging behavior.
//
// If you want to set non-default values for any of these properties, create a builder with
// ldcomponents.Logging(), change its properties with the LoggingConfigurationBuilder methods, and
// store it in Config.Logging:
//
//     config := ld.Config{
//         Logging: ldcomponents.Logging().MinLevel(ldlog.Warn),
//     }
type LoggingConfigurationBuilder struct {
	config internal.LoggingConfigurationImpl
}

// Logging returns a configuration builder for the SDK's logging configuration.
//
// The default configuration has logging enabled with default settings. If you want to set non-default
// values for any of these properties, create a builder with ldcomponents.Logging(), change its properties
// with the LoggingConfigurationBuilder methods, and store it in Config.Logging:
//
//     config := ld.Config{
//         Logging: ldcomponents.Logging().MinLevel(ldlog.Warn),
//     }
func Logging() *LoggingConfigurationBuilder {
	return &LoggingConfigurationBuilder{config: internal.LoggingConfigurationImpl{Loggers: ldlog.NewDefaultLoggers()}}
}

// LogEvaluationErrors sets whether the client should log a warning message whenever a flag cannot be evaluated due
// to an error (e.g. there is no flag with that key, or the user properties are invalid). By default, these messages
// are not logged, although you can detect such errors programmatically using the VariationDetail methods.
func (b *LoggingConfigurationBuilder) LogEvaluationErrors(logEvaluationErrors bool) *LoggingConfigurationBuilder {
	b.config.LogEvaluationErrors = logEvaluationErrors
	return b
}

// LogUserKeyInErrors sets whether log messages for errors related to a specific user can include the user key. By
// default, they will not, since the user key might be considered privileged information.
func (b *LoggingConfigurationBuilder) LogUserKeyInErrors(logUserKeyInErrors bool) *LoggingConfigurationBuilder {
	b.config.LogUserKeyInErrors = logUserKeyInErrors
	return b
}

// Loggers specifies an instance of ldlog.Loggers to use for SDK logging. The ldlog package contains
// methods for customizing the destination and level filtering of log output.
func (b *LoggingConfigurationBuilder) Loggers(loggers ldlog.Loggers) *LoggingConfigurationBuilder {
	b.config.Loggers = loggers
	return b
}

// MinLevel specifies the minimum level for log output, where ldlog.Debug is the lowest and ldlog.Error
// is the highest. Log messages at a level lower than this will be suppressed. The default is
// ldlog.Info.
//
// This is equivalent to creating an ldlog.Loggers instance, calling SetMinLevel() on it, and then
// passing it to LoggingConfigurationBuilder.Loggers().
func (b *LoggingConfigurationBuilder) MinLevel(level ldlog.LogLevel) *LoggingConfigurationBuilder {
	b.config.Loggers.SetMinLevel(level)
	return b
}

// CreateLoggingConfiguration is called internally by the SDK.
func (b *LoggingConfigurationBuilder) CreateLoggingConfiguration() interfaces.LoggingConfiguration {
	return b.config
}

// NoLogging returns a configuration object that disables logging.
//
//     config := ld.Config{
//         Logging: ldcomponents.NoLogging(),
//     }
func NoLogging() interfaces.LoggingConfigurationFactory {
	return noLoggingConfigurationFactory{}
}

type noLoggingConfigurationFactory struct{}

func (f noLoggingConfigurationFactory) CreateLoggingConfiguration() interfaces.LoggingConfiguration {
	return internal.LoggingConfigurationImpl{Loggers: ldlog.NewDisabledLoggers()}
}

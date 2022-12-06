package ldcomponents

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v6/internal"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// LoggingConfigurationBuilder contains methods for configuring the SDK's logging behavior.
//
// If you want to set non-default values for any of these properties, create a builder with
// ldcomponents.[Logging](), change its properties with the LoggingConfigurationBuilder methods, and
// store it in the Logging field of [github.com/launchdarkly/go-server-sdk/v6.Config]:
//
//	config := ld.Config{
//	    Logging: ldcomponents.Logging().MinLevel(ldlog.Warn),
//	}
type LoggingConfigurationBuilder struct {
	inited bool
	config subsystems.LoggingConfiguration
}

// DefaultLogDataSourceOutageAsErrorAfter is the default value for
// [LoggingConfigurationBuilder.LogDataSourceOutageAsErrorAfter]: one minute.
const DefaultLogDataSourceOutageAsErrorAfter = time.Minute

// Logging returns a configuration builder for the SDK's logging configuration.
//
// The default configuration has logging enabled with default settings. If you want to set non-default
// values for any of these properties, create a builder with ldcomponents.Logging(), change its properties
// with the [LoggingConfigurationBuilder] methods, and store it in Config.Logging:
//
//	config := ld.Config{
//	    Logging: ldcomponents.Logging().MinLevel(ldlog.Warn),
//	}
func Logging() *LoggingConfigurationBuilder {
	return &LoggingConfigurationBuilder{}
}

func (b *LoggingConfigurationBuilder) checkValid() bool {
	if b == nil {
		internal.LogErrorNilPointerMethod("LoggingConfigurationBuilder")
		return false
	}
	if !b.inited {
		b.config = subsystems.LoggingConfiguration{
			LogDataSourceOutageAsErrorAfter: DefaultLogDataSourceOutageAsErrorAfter,
			Loggers:                         ldlog.NewDefaultLoggers(),
		}
		b.inited = true
	}
	return true
}

// LogDataSourceOutageAsErrorAfter sets the time threshold, if any, after which the SDK will log a data
// source outage at Error level instead of Warn level.
//
// A data source outage means that an error condition, such as a network interruption or an error from
// the LaunchDarkly service, is preventing the SDK from receiving feature flag updates. Many outages are
// brief and the SDK can recover from them quickly; in that case it may be undesirable to log an
// Error line, which might trigger an unwanted automated alert depending on your monitoring
// tools. So, by default, the SDK logs such errors at Warn level. However, if the amount of time
// specified by this method elapses before the data source starts working again, the SDK will log an
// additional message at Error level to indicate that this is a sustained problem.
//
// The default is [DefaultLogDataSourceOutageAsErrorAfter] (one minute). Setting it to zero will disable
// this feature, so you will only get Warn messages.
func (b *LoggingConfigurationBuilder) LogDataSourceOutageAsErrorAfter(
	logDataSourceOutageAsErrorAfter time.Duration,
) *LoggingConfigurationBuilder {
	if b.checkValid() {
		b.config.LogDataSourceOutageAsErrorAfter = logDataSourceOutageAsErrorAfter
	}
	return b
}

// LogEvaluationErrors sets whether the client should log a warning message whenever a flag cannot be evaluated due
// to an error (e.g. there is no flag with that key, or the context properties are invalid). By default, these messages
// are not logged, although you can detect such errors programmatically using the VariationDetail methods. The only
// exception is that the SDK will always log any error involving invalid flag data, because such data should not be
// possible and indicates that LaunchDarkly support assistance may be required.
func (b *LoggingConfigurationBuilder) LogEvaluationErrors(logEvaluationErrors bool) *LoggingConfigurationBuilder {
	if b.checkValid() {
		b.config.LogEvaluationErrors = logEvaluationErrors
	}
	return b
}

// LogContextKeyInErrors sets whether log messages for errors related to a specific evaluation context can include the
// context key. By default, they will not, since the key might be considered privileged information.
func (b *LoggingConfigurationBuilder) LogContextKeyInErrors(logContextKeyInErrors bool) *LoggingConfigurationBuilder {
	if b.checkValid() {
		b.config.LogContextKeyInErrors = logContextKeyInErrors
	}
	return b
}

// Loggers specifies an instance of [ldlog.Loggers] to use for SDK logging. The ldlog package contains
// methods for customizing the destination and level filtering of log output.
func (b *LoggingConfigurationBuilder) Loggers(loggers ldlog.Loggers) *LoggingConfigurationBuilder {
	if b.checkValid() {
		b.config.Loggers = loggers
	}
	return b
}

// MinLevel specifies the minimum level for log output, where [ldlog.Debug] is the lowest and [ldlog.Error]
// is the highest. Log messages at a level lower than this will be suppressed. The default is
// [ldlog.Info].
//
// This is equivalent to creating an ldlog.Loggers instance, calling SetMinLevel() on it, and then
// passing it to LoggingConfigurationBuilder.Loggers().
func (b *LoggingConfigurationBuilder) MinLevel(level ldlog.LogLevel) *LoggingConfigurationBuilder {
	if b.checkValid() {
		b.config.Loggers.SetMinLevel(level)
	}
	return b
}

// Build is called internally by the SDK.
func (b *LoggingConfigurationBuilder) Build(
	clientContext subsystems.ClientContext,
) (subsystems.LoggingConfiguration, error) {
	if !b.checkValid() {
		defaults := LoggingConfigurationBuilder{}
		return defaults.Build(clientContext)
	}
	return b.config, nil
}

// NoLogging returns a configuration object that disables logging.
//
//	config := ld.Config{
//	    Logging: ldcomponents.NoLogging(),
//	}
func NoLogging() subsystems.ComponentConfigurer[subsystems.LoggingConfiguration] {
	// Note that we're using the builder type but returning it as just an opaque ComponentConfigurer, so
	// you can't do something illogical like call MinLevel on it.
	return Logging().Loggers(ldlog.NewDisabledLoggers())
}

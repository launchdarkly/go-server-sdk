package ldai

import (
	"maps"

	"slices"

	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"
)

// Config represents an AI Config.
type Config struct {
	c              datamodel.Config
	trackerFactory func() *Tracker
}

// VariationKey is used internally by LaunchDarkly.
func (c *Config) VariationKey() string {
	return c.c.Meta.VariationKey
}

// Version is used internally by LaunchDarkly.
func (c *Config) Version() int {
	if c.c.Meta.Version == nil {
		return 1
	}
	return *c.c.Meta.Version
}

// Messages returns the messages defined by the config. The series of messages may be
// passed to an AI model provider.
func (c *Config) Messages() []datamodel.Message {
	return slices.Clone(c.c.Messages)
}

// Enabled returns whether the config is enabled.
func (c *Config) Enabled() bool {
	return c.c.Meta.Enabled
}

// ProviderName returns the provider name associated with the config.
func (c *Config) ProviderName() string {
	return c.c.Provider.Name
}

// ModelName returns the model name associated with the config.
func (c *Config) ModelName() string {
	return c.c.Model.Name
}

// ModelParam returns the model parameter named by key. The second parameter is true if the key exists.
func (c *Config) ModelParam(key string) (ldvalue.Value, bool) {
	val, ok := c.c.Model.Parameters[key]
	return val, ok
}

// CustomModelParam returns the custom model parameter named by key. The second parameter is true if the key exists.
func (c *Config) CustomModelParam(key string) (ldvalue.Value, bool) {
	val, ok := c.c.Model.Custom[key]
	return val, ok
}

// Mode returns the AI Config mode (e.g., "completion", "agent", "judge").
func (c *Config) Mode() string {
	return c.c.Mode
}

// EvaluationMetricKey returns the evaluation metric key for judge mode configs.
func (c *Config) EvaluationMetricKey() string {
	return c.c.EvaluationMetricKey
}

// EvaluationMetricKeys returns the deprecated array of evaluation metric keys.
// Use EvaluationMetricKey instead.
func (c *Config) EvaluationMetricKeys() []string {
	return slices.Clone(c.c.EvaluationMetricKeys)
}

// JudgeConfiguration returns the judge configuration attached to this config, if any.
// Returns a defensive copy to prevent mutations.
func (c *Config) JudgeConfiguration() *datamodel.JudgeConfiguration {
	if c.c.JudgeConfiguration == nil {
		return nil
	}
	return &datamodel.JudgeConfiguration{
		Judges: slices.Clone(c.c.JudgeConfiguration.Judges),
	}
}

// CreateTracker returns a new Tracker for a fresh AI run. Each call mints
// a new runId (a UUIDv4) that LaunchDarkly uses to correlate the run's
// events in metrics views. Call this once per AI run; metrics from
// different runIds cannot be combined.
//
// Returns nil if the config was not obtained via the Client.
func (c *Config) CreateTracker() *Tracker {
	if c.trackerFactory == nil {
		return nil
	}
	return c.trackerFactory()
}

// AsLdValue is used internally.
func (c *Config) AsLdValue() ldvalue.Value {
	return ldvalue.FromJSONMarshal(c.c)
}

// ConfigBuilder is used to define a default AI Config, returned when LaunchDarkly is unreachable or there
// is an error evaluating the Config.
type ConfigBuilder struct {
	messages             []datamodel.Message
	enabled              bool
	providerName         string
	modelName            string
	modelParams          map[string]ldvalue.Value
	modelCustomParams    map[string]ldvalue.Value
	mode                 string
	evaluationMetricKey  string
	evaluationMetricKeys []string
	judgeConfiguration   *datamodel.JudgeConfiguration
}

// NewConfig returns a new ConfigBuilder. By default, the Config is disabled.
func NewConfig() *ConfigBuilder {
	return &ConfigBuilder{
		modelParams:       make(map[string]ldvalue.Value),
		modelCustomParams: make(map[string]ldvalue.Value),
	}
}

// Disabled is a helper that returns a built Config that is disabled and contains no messages.
func Disabled() Config {
	return NewConfig().Disable().Build()
}

// WithMessage appends a message to the config with the given role.
func (cb *ConfigBuilder) WithMessage(content string, role datamodel.Role) *ConfigBuilder {
	cb.messages = append(cb.messages, datamodel.Message{
		Content: content,
		Role:    role,
	})
	return cb
}

// WithEnabled sets whether the config is enabled. See also Enable and Disable.
func (cb *ConfigBuilder) WithEnabled(enabled bool) *ConfigBuilder {
	cb.enabled = enabled
	return cb
}

// Enable enables the config.
func (cb *ConfigBuilder) Enable() *ConfigBuilder {
	return cb.WithEnabled(true)
}

// Disable disables the config.
func (cb *ConfigBuilder) Disable() *ConfigBuilder {
	return cb.WithEnabled(false)
}

// WithModelName sets the model name associated with the config.
func (cb *ConfigBuilder) WithModelName(modelName string) *ConfigBuilder {
	cb.modelName = modelName
	return cb
}

// WithProviderName sets the provider name associated with the config.
func (cb *ConfigBuilder) WithProviderName(providerName string) *ConfigBuilder {
	cb.providerName = providerName
	return cb
}

// WithModelParam sets a model parameter named by key to the given value. If the key already exists, it will be
// overwritten. Model parameters are generally set by LaunchDarkly; for custom parameters not recognized by
// LaunchDarkly, use WithModelCustomParam.
func (cb *ConfigBuilder) WithModelParam(key string, value ldvalue.Value) *ConfigBuilder {
	cb.modelParams[key] = value
	return cb
}

// WithCustomModelParam sets a custom model parameter named by key to the given value. If the key already exists, it
// will be overwritten.
func (cb *ConfigBuilder) WithCustomModelParam(key string, value ldvalue.Value) *ConfigBuilder {
	cb.modelCustomParams[key] = value
	return cb
}

// WithMode sets the AI Config mode (e.g., "completion", "agent", "judge").
func (cb *ConfigBuilder) WithMode(mode string) *ConfigBuilder {
	cb.mode = mode
	return cb
}

// WithEvaluationMetricKey sets the evaluation metric key for judge mode configs.
func (cb *ConfigBuilder) WithEvaluationMetricKey(key string) *ConfigBuilder {
	cb.evaluationMetricKey = key
	return cb
}

// WithEvaluationMetricKeys sets the deprecated array of evaluation metric keys.
// Use WithEvaluationMetricKey instead.
func (cb *ConfigBuilder) WithEvaluationMetricKeys(keys []string) *ConfigBuilder {
	cb.evaluationMetricKeys = slices.Clone(keys)
	return cb
}

// WithJudgeConfiguration sets the judge configuration for this config.
// The provided judgeConfig is defensively copied.
func (cb *ConfigBuilder) WithJudgeConfiguration(judgeConfig *datamodel.JudgeConfiguration) *ConfigBuilder {
	if judgeConfig == nil {
		cb.judgeConfiguration = nil
		return cb
	}
	cb.judgeConfiguration = &datamodel.JudgeConfiguration{
		Judges: slices.Clone(judgeConfig.Judges),
	}
	return cb
}

// Build creates a Config from the current builder state.
func (cb *ConfigBuilder) Build() Config {
	var judgeConfig *datamodel.JudgeConfiguration
	if cb.judgeConfiguration != nil {
		judgeConfig = &datamodel.JudgeConfiguration{
			Judges: slices.Clone(cb.judgeConfiguration.Judges),
		}
	}

	return Config{
		c: datamodel.Config{
			Messages: slices.Clone(cb.messages),
			Meta: datamodel.Meta{
				Enabled: cb.enabled,
			},
			Model: datamodel.Model{
				Name:       cb.modelName,
				Parameters: maps.Clone(cb.modelParams),
				Custom:     maps.Clone(cb.modelCustomParams),
			},
			Provider: datamodel.Provider{
				Name: cb.providerName,
			},
			Mode:                 cb.mode,
			EvaluationMetricKey:  cb.evaluationMetricKey,
			EvaluationMetricKeys: slices.Clone(cb.evaluationMetricKeys),
			JudgeConfiguration:   judgeConfig,
		},
	}
}

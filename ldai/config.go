package ldai

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai/internal/datamodel"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// Config represents an AI config.
type Config struct {
	c datamodel.Config
}

// VersionKey is used internally by LaunchDarkly.
func (c *Config) VersionKey() string {
	return c.c.Meta.VersionKey
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

// ProviderId returns the provider ID associated with the config.
func (c *Config) ProviderId() string {
	return c.c.Provider.Id
}

// ModelId returns the model ID associated with the config.
func (c *Config) ModelId() string {
	return c.c.Model.Id
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

// AsLdValue is used internally.
func (c *Config) AsLdValue() ldvalue.Value {
	return ldvalue.FromJSONMarshal(c.c)
}

// ConfigBuilder is used to define a default AI config, returned when LaunchDarkly is unreachable or there
// is an error evaluating the config.
type ConfigBuilder struct {
	messages          []datamodel.Message
	enabled           bool
	providerId        string
	modelId           string
	modelParams       map[string]ldvalue.Value
	modelCustomParams map[string]ldvalue.Value
}

// NewConfig returns a new ConfigBuilder. By default, the config is disabled.
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

// WithModelId sets the model ID associated with the config.
func (cb *ConfigBuilder) WithModelId(modelId string) *ConfigBuilder {
	cb.modelId = modelId
	return cb
}

// WithProviderId sets the provider ID associated with the config.
func (cb *ConfigBuilder) WithProviderId(providerId string) *ConfigBuilder {
	cb.providerId = providerId
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

// Build creates a Config from the current builder state.
func (cb *ConfigBuilder) Build() Config {
	return Config{
		c: datamodel.Config{
			Messages: slices.Clone(cb.messages),
			Meta: datamodel.Meta{
				Enabled: cb.enabled,
			},
			Model: datamodel.Model{
				Id:         cb.modelId,
				Parameters: maps.Clone(cb.modelParams),
				Custom:     maps.Clone(cb.modelCustomParams),
			},
			Provider: datamodel.Provider{
				Id: cb.providerId,
			},
		},
	}
}

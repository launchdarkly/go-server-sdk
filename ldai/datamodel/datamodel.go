package datamodel

import "github.com/launchdarkly/go-sdk-common/v3/ldvalue"

// Meta defines the serialization format for config metadata.
type Meta struct {
	// VariationKey is the variation key.
	VariationKey string `json:"variationKey,omitempty"`

	// Enabled is true if the config is enabled.
	Enabled bool `json:"enabled,omitempty"`

	// Version is the version of the Variation.
	Version int `json:"version,omitempty"`
}

// Model defines the serialization format for a model.
type Model struct {
	// Name identifies the model.
	Name string `json:"name"`

	// Parameters are the model parameters, generally provided by LaunchDarkly.
	Parameters map[string]ldvalue.Value `json:"parameters,omitempty"`

	// Custom are custom model parameters, generally provided by the user.
	Custom map[string]ldvalue.Value `json:"custom,omitempty"`
}

// Provider defines the serialization format for a model provider.
type Provider struct {
	// Name identifies the provider.
	Name string `json:"name"`
}

// Role defines the role of a message.
type Role string

const (
	// User represents the user.
	User Role = "user"

	// System represents the system.
	System Role = "system"

	// Assistant represents an assistant.
	Assistant Role = "assistant"
)

// Message defines the serialization format for a message which may be passed to an AI model provider.
type Message struct {
	// Content is the message content.
	Content string `json:"content"`

	// Role is the role of the message.
	Role Role `json:"role"`
}

// Config defines the serialization format for an AI config.
type Config struct {
	// Messages is a list of messages. The messages received from LaunchDarkly are uninterpolated.
	Messages []Message `json:"messages,omitempty"`

	// Meta is the config metadata.
	Meta Meta `json:"_ldMeta,omitempty"`

	// Model is the model.
	Model Model `json:"model,omitempty"`

	// Provider is the provider.
	Provider Provider `json:"provider,omitempty"`
}

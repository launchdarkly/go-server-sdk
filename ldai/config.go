package ldai

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai/internal/datamodel"
	"golang.org/x/exp/slices"
)

type Message struct {
	content string
	role    datamodel.Role
}

func (m Message) Content() string {
	return m.content
}

func (m Message) Role() datamodel.Role {
	return m.role
}

type Config struct {
	c datamodel.Config
}

func (c *Config) VersionKey() string {
	return c.c.Meta.VersionKey
}

func (c *Config) Messages() []Message {
	var messages []Message
	for _, m := range c.c.Messages {
		messages = append(messages, Message{
			content: m.Content,
			role:    m.Role,
		})
	}
	return messages
}

func (c *Config) Enabled() bool {
	return c.c.Meta.Enabled
}

func (c *Config) ProviderId() string {
	return c.c.Provider.Id
}

func (c *Config) ModelId() string {
	return c.c.Model.Id
}

func (c *Config) AsLdValue() ldvalue.Value {
	return ldvalue.FromJSONMarshal(c.c)
}

type ConfigBuilder struct {
	messages   []datamodel.Message
	enabled    bool
	providerId string
	modelId    string
}

func NewConfig() *ConfigBuilder {
	return &ConfigBuilder{}
}

func Disabled() Config {
	return NewConfig().Disable().Build()
}

func (cb *ConfigBuilder) WithMessage(content string, role datamodel.Role) *ConfigBuilder {
	cb.messages = append(cb.messages, datamodel.Message{
		Content: content,
		Role:    role,
	})
	return cb
}

func (cb *ConfigBuilder) WithEnabled(enabled bool) *ConfigBuilder {
	cb.enabled = enabled
	return cb
}

func (cb *ConfigBuilder) Enable() *ConfigBuilder {
	return cb.WithEnabled(true)
}

func (cb *ConfigBuilder) Disable() *ConfigBuilder {
	return cb.WithEnabled(false)
}

func (cb *ConfigBuilder) WithModelId(modelId string) *ConfigBuilder {
	cb.modelId = modelId
	return cb
}

func (cb *ConfigBuilder) WithProviderId(providerId string) *ConfigBuilder {
	cb.providerId = providerId
	return cb
}

func (cb *ConfigBuilder) Build() Config {
	return Config{
		c: datamodel.Config{
			Messages: slices.Clone(cb.messages),
			Meta: datamodel.Meta{
				Enabled: cb.enabled,
			},
			Model: datamodel.Model{
				Id: cb.modelId,
			},
			Provider: datamodel.Provider{
				Id: cb.providerId,
			},
		},
	}
}

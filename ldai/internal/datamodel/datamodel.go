package datamodel

import "github.com/launchdarkly/go-sdk-common/v3/ldvalue"

type Meta struct {
	VersionKey string `json:"versionKey,omitempty"`
	Enabled    bool   `json:"enabled,omitempty"`
}

type Model struct {
	Id         string                   `json:"id,omitempty"`
	Parameters map[string]ldvalue.Value `json:"parameters,omitempty"`
	Custom     map[string]ldvalue.Value `json:"custom,omitempty"`
}

type Provider struct {
	Id string `json:"id,omitempty"`
}

type Role string

const (
	User      Role = "user"
	System    Role = "system"
	Assistant Role = "assistant"
)

type Message struct {
	Content string `json:"content"`
	Role    Role   `json:"role"`
}

type Config struct {
	Messages []Message `json:"messages,omitempty"`
	Meta     Meta      `json:"_ldMeta,omitempty"`
	Model    Model     `json:"model,omitempty"`
	Provider Provider  `json:"provider,omitempty"`
}

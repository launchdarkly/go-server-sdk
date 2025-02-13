package ldbuilders

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// ArbitraryConfigsBuilder provides a builder pattern for ldmodel.ArbitraryConfigs.
type ArbitraryConfigsBuilder struct {
	config ldmodel.ArbitraryConfigs
}

// NewArbitraryConfigsBuilder creates a new ArbitraryConfigsBuilder.
func NewArbitraryConfigsBuilder(key string) *ArbitraryConfigsBuilder {
	return &ArbitraryConfigsBuilder{
		config: ldmodel.ArbitraryConfigs{
			Key: key,
		},
	}
}

// Build returns the configured ArbitraryConfigs.
func (b *ArbitraryConfigsBuilder) Build() ldmodel.ArbitraryConfigs {
	return b.config
}

// DataType sets the data type of the configuration.
func (b *ArbitraryConfigsBuilder) DataType(dataType ldmodel.ArbitraryConfigType) *ArbitraryConfigsBuilder {
	b.config.DataType = dataType
	return b
}

// Values sets the configuration values.
func (b *ArbitraryConfigsBuilder) Values(values ldvalue.Value) *ArbitraryConfigsBuilder {
	b.config.Values = values
	return b
}

// Version sets the version of the configuration.
func (b *ArbitraryConfigsBuilder) Version(version int) *ArbitraryConfigsBuilder {
	b.config.Version = version
	return b
}

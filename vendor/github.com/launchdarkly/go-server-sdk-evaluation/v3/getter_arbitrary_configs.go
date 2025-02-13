package evaluation

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

type arbitraryConfigProvider struct {
}

func (p *arbitraryConfigProvider) GetArbitraryConfigMapValues(config ldmodel.ArbitraryConfigs, key any) ldvalue.Value {
	if config.Key == "" || config.Values == nil || config.DataType != ldmodel.KeyValuesType {
		return ldvalue.Null()
	}
	values := config.Values.(map[string]any)
	value, ok := values[key.(string)]
	if !ok {
		return ldvalue.Null()
	}
	return ldvalue.FromJSONMarshal(value)
}

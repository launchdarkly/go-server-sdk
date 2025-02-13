package evaluation

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

type arbitraryConfigProvider struct {
}

func (p *arbitraryConfigProvider) GetArbitraryConfigMapValues(config ldmodel.ArbitraryConfigs, key ldvalue.Value) ldvalue.Value {
	if config.Key == "" || config.DataType != ldmodel.KeyValuesType {
		return ldvalue.Null()
	}
	values := config.Values.AsValueMap()
	if !values.IsDefined() {
		return ldvalue.Null()
	}
	return values.Get(key.String())
}

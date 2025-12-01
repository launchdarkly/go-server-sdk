package subsystems

import (
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

type kindMap map[datakinds.DataKindInternal][]ldstoretypes.KeyedItemDescriptor

func (k kindMap) flatten() (result []ldstoretypes.Collection) {
	for kind, items := range k {
		result = append(result, ldstoretypes.Collection{
			Kind:  kind,
			Items: items,
		})
	}
	return
}

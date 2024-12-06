package fdv2proto

import (
	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// ToStorableItems converts a list of FDv2 events to a list of collections suitable for insertion
// into a data store.
func ToStorableItems(deltas []Change) ([]ldstoretypes.Collection, error) {
	collections := make(kindMap)
	for _, event := range deltas {
		kind, ok := event.Kind.ToFDV1()
		if !ok {
			// If we don't recognize this kind, it's not an error and should be ignored for forwards
			// compatibility.
			continue
		}

		switch event.Action {
		case ChangeTypePut:
			// A put requires deserializing the item. We delegate to the optimized streaming JSON
			// parser.
			reader := jreader.NewReader(event.Object)
			item, err := kind.DeserializeFromJSONReader(&reader)
			if err != nil {
				return nil, err
			}
			collections[kind] = append(collections[kind], ldstoretypes.KeyedItemDescriptor{
				Key:  event.Key,
				Item: item,
			})
		case ChangeTypeDelete:
			// A deletion is represented by a tombstone, which is an ItemDescriptor with a version and nil item.
			collections[kind] = append(collections[kind], ldstoretypes.KeyedItemDescriptor{
				Key:  event.Key,
				Item: ldstoretypes.ItemDescriptor{Version: event.Version, Item: nil},
			})
		default:
			// An unknown action isn't an error, and should be ignored for forwards compatibility.
			continue
		}
	}

	return collections.flatten(), nil
}

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

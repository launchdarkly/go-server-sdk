package overrides

import (
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// Overlay merges an override Layer over a base store: a read for a key returns the override
// entry when one exists and the base entry otherwise. Placing the overlay at the store read
// boundary is what makes targeting rules, prerequisites, and segment matches behave
// identically for overridden and ordinary data — they are the same reads through the same
// boundary.
type Overlay struct {
	base  subsystems.ReadOnlyStore
	layer *Layer
}

var _ subsystems.ReadOnlyStore = (*Overlay)(nil)

// NewOverlay creates an Overlay over the given base store and layer.
func NewOverlay(base subsystems.ReadOnlyStore, layer *Layer) *Overlay {
	return &Overlay{base: base, layer: layer}
}

// Get returns the override entry for the key if one exists, and otherwise delegates to the
// base store. This works even when the base store is uninitialized, because an uninitialized
// base reports not-found rather than failing.
func (o *Overlay) Get(kind st.DataKind, key string) (st.ItemDescriptor, error) {
	if item, ok := o.layer.Get(kind, key); ok {
		return item, nil
	}
	return o.base.Get(kind, key)
}

// GetAll returns the union of the base store's items and the layer's items, with the
// override entry winning for any key present in both (including keys the base holds as
// deleted-item tombstones).
func (o *Overlay) GetAll(kind st.DataKind) ([]st.KeyedItemDescriptor, error) {
	baseItems, err := o.base.GetAll(kind)
	if err != nil {
		return nil, err
	}
	overrideItems := o.layer.All(kind)
	if len(overrideItems) == 0 {
		return baseItems, nil
	}

	result := make([]st.KeyedItemDescriptor, 0, len(baseItems)+len(overrideItems))
	seen := make(map[string]bool, len(baseItems))
	for _, item := range baseItems {
		if overrideItem, ok := overrideItems[item.Key]; ok {
			item.Item = overrideItem
		}
		seen[item.Key] = true
		result = append(result, item)
	}
	for key, item := range overrideItems {
		if !seen[key] {
			result = append(result, st.KeyedItemDescriptor{Key: key, Item: item})
		}
	}
	return result, nil
}

// IsInitialized delegates to the base store: the override layer never affects
// initialization status or data availability.
func (o *Overlay) IsInitialized() bool {
	return o.base.IsInitialized()
}

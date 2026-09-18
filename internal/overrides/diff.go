package overrides

import (
	"bytes"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/toposort"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

var diffKinds = []st.DataKind{datakinds.Features, datakinds.Segments} //nolint:gochecknoglobals

// computeAffectedFlags returns the keys of all flags whose merged-view evaluation may have
// changed when the override layer was replaced. The result includes the flags whose override
// entries were added, removed, or changed. Dependency fan-out adds every flag that depends,
// directly or transitively, on any added, removed, or changed entry of either kind.
func computeAffectedFlags(
	oldOverrides, newOverrides layerContents,
	oldMerged, newMerged mergedView,
) []string {
	seeds := diffOverrides(oldOverrides, newOverrides)
	if len(seeds) == 0 {
		return nil
	}

	// Dependency edges are computed over both the old and the new merged views, because a
	// replacement can rewire dependencies. For example, removing a flag override restores the
	// prerequisite edges of the LaunchDarkly definition. Flags that depended on the override's
	// references exist as dependents only in the old view.
	oldTracker := newTrackerFromView(oldMerged)
	newTracker := newTrackerFromView(newMerged)
	affected := make(toposort.Neighbors)
	for _, seed := range seeds {
		oldTracker.AddAffectedItems(affected, seed)
		newTracker.AddAffectedItems(affected, seed)
	}

	var flagKeys []string
	for vertex := range affected {
		if vertex.Kind() == datakinds.Features {
			flagKeys = append(flagKeys, vertex.Key())
		}
	}
	return flagKeys
}

// diffOverrides returns a vertex for each key whose override entry differs between the two
// layer snapshots. An added or removed entry is always a change, even when its content
// matches the underlying LaunchDarkly data. The override marker alone changes the served
// entry. Entries present in both snapshots are compared by their serialized form. The layer
// is rebuilt wholesale on every update, so pointer or version comparison would report every
// retained entry as changed.
func diffOverrides(oldOverrides, newOverrides layerContents) []toposort.Vertex {
	var seeds []toposort.Vertex
	for _, kind := range diffKinds {
		oldItems := oldOverrides[kind]
		newItems := newOverrides[kind]
		for key, oldItem := range oldItems {
			newItem, inNew := newItems[key]
			if !inNew || !itemsEqual(kind, oldItem, newItem) {
				seeds = append(seeds, toposort.NewVertex(kind, key))
			}
		}
		for key := range newItems {
			if _, inOld := oldItems[key]; !inOld {
				seeds = append(seeds, toposort.NewVertex(kind, key))
			}
		}
	}
	return seeds
}

func itemsEqual(kind st.DataKind, a, b st.ItemDescriptor) bool {
	if a.Version != b.Version {
		return false
	}
	return bytes.Equal(kind.Serialize(a), kind.Serialize(b))
}

// mergedView is a snapshot of the data visible at the store read boundary: base data with
// override entries overlaid.
type mergedView map[st.DataKind]map[string]st.ItemDescriptor

// snapshotMergedView captures the merged view of a base store and a layer snapshot. A base
// read failure for a kind yields just the overrides for that kind. This degrades the
// dependency fan-out but never loses the directly changed keys.
func snapshotMergedView(base interface {
	GetAll(st.DataKind) ([]st.KeyedItemDescriptor, error)
}, overrides layerContents) mergedView {
	view := mergedView{}
	for _, kind := range diffKinds {
		items := map[string]st.ItemDescriptor{}
		if baseItems, err := base.GetAll(kind); err == nil {
			for _, item := range baseItems {
				items[item.Key] = item.Item
			}
		}
		for key, item := range overrides[kind] {
			items[key] = item
		}
		view[kind] = items
	}
	return view
}

func newTrackerFromView(view mergedView) *toposort.DependencyTracker {
	tracker := toposort.NewDependencyTracker()
	for _, kind := range diffKinds {
		for key, item := range view[kind] {
			tracker.UpdateDependenciesFrom(kind, key, item)
		}
	}
	return tracker
}

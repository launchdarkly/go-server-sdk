// Package overrides implements the flag/segment override layer. The layer is a runtime-mutable
// collection of flag and segment definitions, supplied by an override source. Those
// definitions take precedence over LaunchDarkly data at evaluation time.
package overrides

import (
	"sync"
	"sync/atomic"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/toposort"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

type layerContents map[st.DataKind]map[string]st.ItemDescriptor

// Layer is a thread-safe store of override entries, replaced wholesale on each update from
// an override source.
type Layer struct {
	mu       sync.RWMutex
	contents layerContents
	nonEmpty atomic.Bool
	// The dependency edges of the layer's own entries: which entries reference which flags and
	// segments. Rebuilt with the contents on every replacement.
	dependencies *toposort.DependencyTracker
}

// NewLayer creates an empty Layer.
func NewLayer() *Layer {
	return &Layer{contents: layerContents{}, dependencies: toposort.NewDependencyTracker()}
}

// SetAll atomically replaces the entire layer contents. An empty or nil slice clears the
// layer.
//
// Each flag or segment is stored as a marked shallow copy. The copy shares its nested slices
// with the caller's entity, and the layer never writes to them. The caller's value is never
// marked. A source may retain the entities it supplied and supply them again. It must not
// modify them after it supplied them.
//
// Returns the previous and new contents. The returned maps must not be modified.
func (l *Layer) SetAll(data []st.Collection) (previous, current layerContents) {
	replacement := layerContents{}
	dependencies := toposort.NewDependencyTracker()
	count := 0
	for _, coll := range data {
		items := make(map[string]st.ItemDescriptor, len(coll.Items))
		for _, item := range coll.Items {
			copied := markedCopy(item.Item)
			items[item.Key] = copied
			dependencies.UpdateDependenciesFrom(coll.Kind, item.Key, copied)
			count++
		}
		replacement[coll.Kind] = items
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	previous = l.contents
	l.contents = replacement
	l.dependencies = dependencies
	l.nonEmpty.Store(count != 0)
	return previous, replacement
}

// AddAffectedItems adds the given item to the set together with every item whose override entry
// depends on it, directly or through other override entries. A LaunchDarkly update uses this so
// that a change to a LaunchDarkly segment or prerequisite notifies the overridden flags that
// reference it, which the data system's own dependency tracking cannot see.
func (l *Layer) AddAffectedItems(itemsOut toposort.Neighbors, item toposort.Vertex) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	l.dependencies.AddAffectedItems(itemsOut, item)
}

// Get returns the override entry for a key, if any.
func (l *Layer) Get(kind st.DataKind, key string) (st.ItemDescriptor, bool) {
	if l.IsEmpty() {
		return st.ItemDescriptor{}, false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	item, ok := l.contents[kind][key]
	return item, ok
}

// All returns the entries of the given kind. The returned map must not be modified.
func (l *Layer) All(kind st.DataKind) map[string]st.ItemDescriptor {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.contents[kind]
}

// IsEmpty reports whether the layer contains no entries. It is a single atomic read, so the
// per-evaluation cost of a configured-but-unpopulated override layer is negligible.
func (l *Layer) IsEmpty() bool {
	return !l.nonEmpty.Load()
}

// markedCopy returns a shallow copy of the entity with the override marker set. The copy
// shares its nested slices with the source entity and does not write to them. Entities from
// ldmodel deserialization or the ldbuilders package carry their preprocessing caches. An
// entity built without them still evaluates correctly, because the evaluator falls back to
// scanning the values.
func markedCopy(item st.ItemDescriptor) st.ItemDescriptor {
	switch entity := item.Item.(type) {
	case *ldmodel.FeatureFlag:
		flag := *entity
		flag.IsOverride = true
		item.Item = &flag
	case *ldmodel.Segment:
		segment := *entity
		segment.IsOverride = true
		item.Item = &segment
	}
	return item
}

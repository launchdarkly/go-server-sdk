// Package overrides implements the flag/segment override layer: a runtime-mutable
// collection of flag and segment definitions, supplied by an override source, that takes
// precedence over LaunchDarkly data at evaluation time.
package overrides

import (
	"sync"
	"sync/atomic"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

type layerContents map[st.DataKind]map[string]st.ItemDescriptor

// Layer is a thread-safe store of override entries, replaced wholesale on each update from
// an override source.
type Layer struct {
	mu       sync.RWMutex
	contents layerContents
	nonEmpty atomic.Bool
}

// NewLayer creates an empty Layer.
func NewLayer() *Layer {
	return &Layer{contents: layerContents{}}
}

// SetAll atomically replaces the entire layer contents; an empty or nil slice clears it.
// Every reader treats the stored entities as immutable, and sources may retain the entities
// they supplied, so each flag or segment is stored as a marked copy rather than marking the
// caller's value. Returns the previous and new contents (the returned maps must not be
// modified).
func (l *Layer) SetAll(data []st.Collection) (previous, current layerContents) {
	replacement := layerContents{}
	count := 0
	for _, coll := range data {
		items := make(map[string]st.ItemDescriptor, len(coll.Items))
		for _, item := range coll.Items {
			items[item.Key] = markedCopy(item.Item)
			count++
		}
		replacement[coll.Kind] = items
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	previous = l.contents
	l.contents = replacement
	l.nonEmpty.Store(count != 0)
	return previous, replacement
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

// HasFlag reports whether the layer contains a flag entry for the given key.
func (l *Layer) HasFlag(key string) bool {
	if l.IsEmpty() {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.contents[datakinds.Features][key]
	return ok
}

// IsEmpty reports whether the layer contains no entries. It is a single atomic read, so the
// per-evaluation cost of a configured-but-unpopulated override layer is negligible.
func (l *Layer) IsEmpty() bool {
	return !l.nonEmpty.Load()
}

// markedCopy returns the item with its entity replaced by a copy carrying the override
// marker. The copies are also re-preprocessed defensively: entities that came from the
// standard deserialization or builders already are, but the sink cannot know how an
// override source constructed them, and preprocessing is idempotent.
func markedCopy(item st.ItemDescriptor) st.ItemDescriptor {
	switch entity := item.Item.(type) {
	case *ldmodel.FeatureFlag:
		flag := *entity
		flag.IsOverride = true
		ldmodel.PreprocessFlag(&flag)
		item.Item = &flag
	case *ldmodel.Segment:
		segment := *entity
		segment.IsOverride = true
		ldmodel.PreprocessSegment(&segment)
		item.Item = &segment
	}
	return item
}

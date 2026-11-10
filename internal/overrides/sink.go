package overrides

import (
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

var _ subsystems.OverrideSink = (*Sink)(nil)

// Sink applies override layer replacements supplied by an override source, and notifies
// flag-change listeners of the flags affected by each replacement.
type Sink struct {
	mu           sync.Mutex
	layer        *Layer
	base         subsystems.ReadOnlyStore
	notify       func(flagKey string)
	hasListeners func() bool
	loggers      ldlog.Loggers
}

// NewSink creates a Sink that writes to the given layer. base is the raw store holding
// LaunchDarkly data, without the overlay: merged-view snapshots for change computation are
// built from it plus the layer. The notify and hasListeners callbacks connect the sink to
// the owner's flag-change broadcaster without this package depending on it.
func NewSink(
	layer *Layer,
	base subsystems.ReadOnlyStore,
	notify func(flagKey string),
	hasListeners func() bool,
	loggers ldlog.Loggers,
) *Sink {
	return &Sink{
		layer:        layer,
		base:         base,
		notify:       notify,
		hasListeners: hasListeners,
		loggers:      loggers,
	}
}

// SetOverrides atomically replaces the entire override layer, then notifies listeners of
// every flag whose merged-view evaluation may have changed. Calls are serialized, so
// overlapping updates from a source cannot interleave.
//
// A snapshot that contains a deleted item is rejected as a whole and logged: the layer stays
// unchanged and no listener is notified. Overrides only add or replace definitions, and a
// deleted item in the layer would hide the LaunchDarkly data for its key.
func (s *Sink) SetOverrides(data []st.Collection) {
	if kind, key, found := findDeletedItem(data); found {
		s.loggers.Errorf("Rejected an override snapshot that contains the deleted %s %q; the overrides in "+
			"effect are unchanged. To remove an override, leave its entry out of the snapshot", kind, key)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Computing affected flags requires snapshots of the merged view before and after the
	// replacement. Skip all of that work when nothing is listening.
	if !s.hasListeners() {
		s.layer.SetAll(data)
		return
	}

	previous, current := s.layer.SetAll(data)
	oldMerged := snapshotMergedView(s.base, previous)
	newMerged := snapshotMergedView(s.base, current)

	affected := computeAffectedFlags(previous, current, oldMerged, newMerged)
	if len(affected) > 0 {
		s.loggers.Debugf("Override update affected %d flag(s)", len(affected))
	}
	for _, key := range affected {
		s.notify(key)
	}
}

// findDeletedItem returns the kind, as "flag" or "segment", and the key of the first deleted
// item in the snapshot.
func findDeletedItem(data []st.Collection) (kind, key string, found bool) {
	for _, coll := range data {
		for _, item := range coll.Items {
			if isDeletedItem(item.Item) {
				return itemKindName(coll.Kind), item.Key, true
			}
		}
	}
	return "", "", false
}

// isDeletedItem reports whether the item is a deleted item: a flag or segment marked as
// deleted, or a descriptor with no item, which is the placeholder the data store uses for a
// deleted item.
func isDeletedItem(item st.ItemDescriptor) bool {
	switch entity := item.Item.(type) {
	case *ldmodel.FeatureFlag:
		return entity == nil || entity.Deleted
	case *ldmodel.Segment:
		return entity == nil || entity.Deleted
	}
	return item.Item == nil
}

func itemKindName(kind st.DataKind) string {
	switch kind {
	case datakinds.Features:
		return "flag"
	case datakinds.Segments:
		return "segment"
	}
	return kind.GetName()
}

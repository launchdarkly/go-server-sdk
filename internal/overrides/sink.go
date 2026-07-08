package overrides

import (
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
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
func (s *Sink) SetOverrides(data []st.Collection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Computing affected flags requires snapshots of the merged view before and after the
	// replacement; skip all of that work when nothing is listening.
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

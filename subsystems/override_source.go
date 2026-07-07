package subsystems

import (
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// OverrideSink receives the contents of the SDK's flag/segment override layer. It is
// implemented by the SDK and passed to an OverrideSource's Start method; override sources
// call it, they do not implement it.
type OverrideSink interface {
	// SetOverrides atomically replaces the entire override layer with the given flag and
	// segment entries; an empty or nil slice clears the layer. Each call is a full snapshot:
	// entries absent from the call are removed from the layer.
	//
	// Collections must use the SDK's standard data kinds ([ldstoreimpl.Features] and
	// [ldstoreimpl.Segments]) with item values of type *ldmodel.FeatureFlag and
	// *ldmodel.Segment respectively. Sources supply ordinary, fully parsed entities, such as
	// those produced by the ldmodel serialization or the ldbuilders package; the SDK itself
	// marks the entries as overrides.
	//
	// SetOverrides is safe to call from any goroutine. Calls are serialized by the SDK, and
	// the new layer contents are visible to evaluations when the call returns.
	SetOverrides(data []ldstoretypes.Collection)
}

// OverrideSource supplies flag and segment overrides that take precedence over LaunchDarkly
// data at evaluation time, on a per-key basis. Overrides exist for resilience during an
// incident: they let an operator force one or more flags to a known state on a running
// client, whether or not the client can reach LaunchDarkly.
//
// An override source is not a data source. It does not participate in the data system's
// initializer/synchronizer pipeline, and the override layer it populates has no effect on
// the client's initialization status, data availability, or data source status.
//
// To configure an override source, use the Overrides method of the data system
// configuration builder in the ldcomponents package.
type OverrideSource interface {
	// Start begins supplying overrides to the sink and returns without blocking on
	// long-running work. Implementations typically perform an initial load synchronously,
	// then push a full replacement snapshot to the sink whenever their backing data
	// changes, until Close is called. A failed load should leave the previously supplied
	// layer untouched by not calling the sink.
	Start(sink OverrideSink)
	// Close stops the source and releases any resources it holds.
	Close() error
}

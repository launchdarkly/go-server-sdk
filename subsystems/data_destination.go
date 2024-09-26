package subsystems

import (
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
)

// DataDestination represents a sink for data obtained from a data source.
// This interface is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type DataDestination interface {
	// SetBasis defines a new basis for the data store. This means the store must
	// be emptied of any existing data before applying the events. This operation should be
	// atomic with respect to any other operations that modify the store.
	//
	// The selector defines the version of the basis.
	//
	// If persist is true, it indicates that the data should be propagated to any connected persistent
	// store.
	SetBasis(events []fdv2proto.Event, selector *fdv2proto.Selector, persist bool)

	// ApplyDelta applies a set of changes to an existing basis. This operation should be atomic with
	// respect to any other operations that modify the store.
	//
	// The selector defines the new version of the basis.
	//
	// If persist is true, it indicates that the changes should be propagated to any connected persistent
	// store.
	ApplyDelta(events []fdv2proto.Event, selector *fdv2proto.Selector, persist bool)
}

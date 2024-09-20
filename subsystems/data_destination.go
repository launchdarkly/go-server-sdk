package subsystems

import (
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// DataDestination represents a sink for data obtained from a data source.
// This interface is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type DataDestination interface {

	// Init overwrites the current contents of the data store with a set of items for each collection.
	//
	// If the underlying data store returns an error during this operation, the SDK will log it,
	// and set the data source state to DataSourceStateInterrupted with an error of
	// DataSourceErrorKindStoreError. It will not return the error to the data source, but will
	// return false to indicate that the operation failed.
	Init(allData []ldstoretypes.Collection, payloadVersion *int) bool

	// Upsert updates or inserts an item in the specified collection. For updates, the object will only be
	// updated if the existing version is less than the new version.
	//
	// To mark an item as deleted, pass an ItemDescriptor with a nil Item and a nonzero version
	// number. Deletions must be versioned so that they do not overwrite a later update in case updates
	// are received out of order.
	//
	// If the underlying data store returns an error during this operation, the SDK will log it,
	// and set the data source state to DataSourceStateInterrupted with an error of
	// DataSourceErrorKindStoreError. It will not return the error to the data source, but will
	// return false to indicate that the operation failed.
	Upsert(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) bool
}

type DataDestination2 interface {
	SetBasis(events []fdv2proto.Event, selector fdv2proto.Selector, persist bool) error
	ApplyDelta(events []fdv2proto.Event, selector fdv2proto.Selector, persist bool) error
}

package subsystems

import (
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastatus"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

type DataDestination interface {
	// Init overwrites the current contents of the data store with a set of items for each collection.
	//
	// If the underlying data store returns an error during this operation, the SDK will log it,
	// and set the data source state to DataSourceStateInterrupted with an error of
	// DataSourceErrorKindStoreError. It will not return the error to the data source, but will
	// return false to indicate that the operation failed.
	Init(allData []ldstoretypes.Collection, status datastatus.DataStatus) bool

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

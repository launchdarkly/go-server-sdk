package subsystems

import "github.com/launchdarkly/go-server-sdk/v6/interfaces"

// DataStoreUpdateSink is an interface that a data store implementation can use to report information
// back to the SDK.
//
// Application code does not need to use this type. It is for data store implementations.
//
// The SDK passes this in the ClientContext when it is creating a data store component.
type DataStoreUpdateSink interface {
	// UpdateStatus informs the SDK of a change in the data store's operational status.
	//
	// This is what makes the status monitoring mechanisms in DataStoreStatusProvider work.
	UpdateStatus(newStatus interfaces.DataStoreStatus)
}

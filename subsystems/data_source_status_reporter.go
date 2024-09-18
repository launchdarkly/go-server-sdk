package subsystems

import (
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

// DataSourceStatusReporter allows a data source to report its status to the SDK.
//
// This interface is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type DataSourceStatusReporter interface {
	// UpdateStatus informs the SDK of a change in the data source's status.
	//
	// Data source implementations should use this method if they have any concept of being in a valid
	// state, a temporarily disconnected state, or a permanently stopped state.
	//
	// If newState is different from the previous state, and/or newError is non-empty, the SDK
	// will start returning the new status (adding a timestamp for the change) from
	// DataSourceStatusProvider.GetStatus(), and will trigger status change events to any
	// registered listeners.
	//
	// A special case is that if newState is DataSourceStateInterrupted, but the previous state was
	// DataSourceStateInitializing, the state will remain at Initializing because Interrupted is
	// only meaningful after a successful startup.
	UpdateStatus(newState interfaces.DataSourceState, newError interfaces.DataSourceErrorInfo)
}

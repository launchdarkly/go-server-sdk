package interfaces

// UnboundedSegmentStoreStatusProvider is an interface for querying the status of an unbounded segment
// store. The unbounded segment store is the component that receives information about unbounded user
// segments, normally from a database populated by the LaunchDarkly Relay Proxy.
//
// An implementation of this interface is returned by LDClient.GetUnboundedSegmentStoreStatusProvider().
// Application code never needs to implement this interface.
//
// There are two ways to interact with the status. One is to simply get the current status; if its
// Available property is true, then the SDK is able to evaluate user membership in unbounded segments,
// and the Stale property indicates whether the data might be out of date.
//
//     status := client.GetUnboundedSegmentStoreStatusProvider().GetStatus()
//
// Second, you can use AddStatusListener to get a channel that provides a status update whenever the
// unbounded segment store has an error or starts working again.
//
//     statusCh := client.GetUnboundedSegmentStoreStatusProvider().AddStatusListener()
//     go func() {
//         for newStatus := range statusCh {
//             log.Printf("unbounded segment store status is now: %+v", newStatus)
//         }
//     }()
type UnboundedSegmentStoreStatusProvider interface {
	// GetStatus returns the current status of the store.
	GetStatus() UnboundedSegmentStoreStatus

	// AddStatusListener subscribes for notifications of status changes. The returned channel will receive a
	// new UnboundedSegmentStoreStatus value for any change in status.
	//
	// Applications may wish to know if there is an outage in the unbounded segment store, or if it has become
	// stale (the Relay Proxy has stopped updating it with new data), since then flag evaluations that reference
	// an unbounded segment might return incorrect values.
	//
	// If the SDK receives an exception while trying to query the unbounded segment store, then it publishes
	// an UnboundedSegmentStoreStatus where Available is false, to indicate that the store appears to be offline.
	// Once it is successful in querying the store's status, it publishes a new status where Available is true.
	//
	// It is the caller's responsibility to consume values from the channel. Allowing values to accumulate in
	// the channel can cause an SDK goroutine to be blocked. If you no longer need the channel, call
	// RemoveStatusListener.
	AddStatusListener() <-chan UnboundedSegmentStoreStatus

	// RemoveStatusListener unsubscribes from notifications of status changes. The specified channel must be
	// one that was previously returned by AddStatusListener(); otherwise, the method has no effect.
	RemoveStatusListener(<-chan UnboundedSegmentStoreStatus)
}

// UnboundedSegmentStoreStatus contains information about the status of an unbouded segment store,
// provided by UnboundedSegmentStoreStatusProvider.
type UnboundedSegmentStoreStatus struct {
	// Available is true if the unbounded segment store is able to respond to queries, so that the SDK
	// can evaluate whether a user is in a segment or not.
	//
	// If this property is false, the store is not able to make queries (for instance, it may not have
	// a valid database connection). In this case, the SDK will treat any reference to an unbounded
	// segment as if no users are included in that segment. Also, the EvaluationReason associated with
	// any flag evaluation that references an unbounded segment when the store is not available will
	// return ldreason.UnboundedSegmentsStoreError from its GetUnboundedSegmentsStatus() method.
	Available bool

	// Stale is true if the unbounded segment store is available, but has not been updated within
	// the amount of time specified by UnboundedSegmentsConfigurationBuilder.StaleTime(). This may
	// indicate that the LaunchDarkly Relay Proxy, which populates the store, has stopped running
	// or has become unable to receive fresh data from LaunchDarkly. Any feature flag evaluations that
	// reference an unbounded segment will be using the last known data, which may be out of date.
	Stale bool
}

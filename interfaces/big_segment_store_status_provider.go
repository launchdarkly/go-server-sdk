package interfaces

// BigSegmentStoreStatusProvider is an interface for querying the status of a big segment store.
// The big segment store is the component that receives information about big segments, normally
// from a database populated by the LaunchDarkly Relay Proxy.
//
// "Big segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
//
// An implementation of this interface is returned by LDClient.GetBigSegmentStoreStatusProvider().
// Application code never needs to implement this interface.
//
// There are two ways to interact with the status. One is to simply get the current status; if its
// Available property is true, then the SDK is able to evaluate user membership in big segments,
// the Stale property indicates whether the data might be out of date.
//
//     status := client.GetBigSegmentStoreStatusProvider().GetStatus()
//
// Second, you can use AddStatusListener to get a channel that provides a status update whenever the
// big segment store has an error or starts working again.
//
//     statusCh := client.GetBigSegmentStoreStatusProvider().AddStatusListener()
//     go func() {
//         for newStatus := range statusCh {
//             log.Printf("big segment store status is now: %+v", newStatus)
//         }
//     }()
type BigSegmentStoreStatusProvider interface {
	// GetStatus returns the current status of the store.
	GetStatus() BigSegmentStoreStatus

	// AddStatusListener subscribes for notifications of status changes. The returned channel will receive a
	// new BigSegmentStoreStatus value for any change in status.
	//
	// Applications may wish to know if there is an outage in the big segment store, or if it has become stale
	// (the Relay Proxy has stopped updating it with new data), since then flag evaluations that reference a
	// big segment might return incorrect values.
	//
	// If the SDK receives an exception while trying to query the big segment store, then it publishes a
	// BigSegmentStoreStatus where Available is false, to indicate that the store appears to be offline. Once
	// it is successful in querying the store's status, it publishes a new status where Available is true.
	//
	// It is the caller's responsibility to consume values from the channel. Allowing values to accumulate in
	// the channel can cause an SDK goroutine to be blocked. If you no longer need the channel, call
	// RemoveStatusListener.
	AddStatusListener() <-chan BigSegmentStoreStatus

	// RemoveStatusListener unsubscribes from notifications of status changes. The specified channel must be
	// one that was previously returned by AddStatusListener(); otherwise, the method has no effect.
	RemoveStatusListener(<-chan BigSegmentStoreStatus)
}

// BigSegmentStoreStatus contains information about the status of a big segment store, provided by
// BigSegmentStoreStatusProvider.
//
// "Big segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
type BigSegmentStoreStatus struct {
	// Available is true if the big segment store is able to respond to queries, so that the SDK can
	// evaluate whether a user is in a segment or not.
	//
	// If this property is false, the store is not able to make queries (for instance, it may not have
	// a valid database connection). In this case, the SDK will treat any reference to a big segment
	// as if no users are included in that segment. Also, the EvaluationReason associated with any
	// flag evaluation that references a big segment when the store is not available will return
	// ldreason.BigSegmentsStoreError from its GetBigSegmentsStatus() method.
	Available bool

	// Stale is true if the big segment store is available, but has not been updated within the amount
	// of time specified by BigSegmentsConfigurationBuilder.StaleTime(). This may indicate that the
	// LaunchDarkly Relay Proxy, which populates the store, has stopped running or has become unable
	// to receive fresh data from LaunchDarkly. Any feature flag evaluations that reference a big
	// segment will be using the last known data, which may be out of date.
	Stale bool
}

package interfaces

import (
	"io"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// UnboundedSegmentsConfiguration encapsulates the SDK's configuration with regard to unbounded
// segments.
//
// See ldcomponents.UnboundedSegmentsConfigurationBuilder for more details on these properties.
type UnboundedSegmentsConfiguration interface {
	// GetStore returns the data store instance that is used for unbounded segments data.
	GetStore() UnboundedSegmentStore

	// GetUserCacheSize returns the value set by UnboundedSegmentsConfigurationBuilder.CacheSize.
	GetUserCacheSize() int

	// GetUserCacheTime returns the value set by UnboundedSegmentsConfigurationBuilder.CacheTime.
	GetUserCacheTime() time.Duration

	// GetStatusPollInterval returns the value set by UnboundedSegmentsConfigurationBuilder.StatusPollInterval.
	GetStatusPollInterval() time.Duration

	// StaleAfter returns the value set by UnboundedSegmentsConfigurationBuilder.StaleAfter.
	GetStaleAfter() time.Duration
}

// UnboundedSegmentsConfigurationFactory is an interface for a factory that creates an UnboundedSegmentsConfiguration.
type UnboundedSegmentsConfigurationFactory interface {
	CreateUnboundedSegmentsConfiguration(context ClientContext) (UnboundedSegmentsConfiguration, error)
}

// UnboundedSegmentStoreFactory is a factory that creates some implementation of UnboundedSegmentStore.
type UnboundedSegmentStoreFactory interface {
	// CreateUnboundedSegmentStore is called by the SDK to create the implementation instance.
	//
	// This happens only when MakeClient or MakeCustomClient is called. The implementation instance
	// is then tied to the life cycle of the LDClient, so it will receive a Close() call when the
	// client is closed.
	//
	// If the factory returns an error, creation of the LDClient fails.
	CreateUnboundedSegmentStore(context ClientContext) (UnboundedSegmentStore, error)
}

// UnboundedSegmentStore is an interface for a read-only data store that allows querying of user
// membership in unbounded segments.
type UnboundedSegmentStore interface {
	io.Closer

	// GetMetadata returns information about the overall state of the store. This method will be
	// called only when the SDK needs the latest state, so it should not be cached.
	GetMetadata() (UnboundedSegmentStoreMetadata, error)

	// GetUserMembership queries the store for a snapshot of the current segment state for a specific
	// user. The userHash is a base64-encoded string produced by hashing the user key as defined by
	// the unbounded segments specification; the store implementation does not need to know the details
	// of how this is done, because it deals only with already-hashed keys, but the string can be
	// assumed to only contain characters that are valid in base64.
	GetUserMembership(userHash string) (UnboundedSegmentMembership, error)
}

// UnboundedSegmentStoreMetadata contains values returned by UnboundedSegmentStore.GetMetadata().
type UnboundedSegmentStoreMetadata struct {
	// LastUpToDate is the timestamp of the last update to the UnboundedSegmentStore. It is zero if
	// the store has never been updated.
	LastUpToDate ldtime.UnixMillisecondTime
}

// UnboundedSegmentMembership is the return type of UnboundedSegmentStore.GetUserMembership(). It
// is associated with a single user, and provides the ability to check whether that user is included in
// or excluded from any number of unbounded segments.
//
// This is an immutable snapshot of the state for this user at the time GetUnboundedSegmentMembership
// was called. Calling CheckMembership should not cause the state to be queried again. The object
// should be safe for concurrent access by multiple goroutines.
type UnboundedSegmentMembership interface {
	// CheckMembership tests whether the user is explicitly included or explicitly excluded in the
	// specified segment, or neither.
	//
	// If the user is explicitly included (regardless of whether the user is also explicitly
	// excluded or not-- that is, inclusion takes priority over exclusion), the method returns an
	// OptionalBool with a true value.
	//
	// If the user is explicitly excluded, and is not explicitly included, the method returns an
	// OptionalBool with a false value.
	//
	// If the user's status in the segment is undefined, the method returns OptionalBool{} with no
	// value (so calling IsDefined() on it will return false).
	CheckMembership(segmentKey string) ldvalue.OptionalBool
}

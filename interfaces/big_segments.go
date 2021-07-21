package interfaces

import (
	"io"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// BigSegmentsConfiguration encapsulates the SDK's configuration with regard to big segments.
//
// "Big segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
//
// See ldcomponents.BigSegmentsConfigurationBuilder for more details on these properties.
type BigSegmentsConfiguration interface {
	// GetStore returns the data store instance that is used for big segments data.
	GetStore() BigSegmentStore

	// GetUserCacheSize returns the value set by BigSegmentsConfigurationBuilder.CacheSize.
	GetUserCacheSize() int

	// GetUserCacheTime returns the value set by BigSegmentsConfigurationBuilder.CacheTime.
	GetUserCacheTime() time.Duration

	// GetStatusPollInterval returns the value set by BigSegmentsConfigurationBuilder.StatusPollInterval.
	GetStatusPollInterval() time.Duration

	// StaleAfter returns the value set by BigSegmentsConfigurationBuilder.StaleAfter.
	GetStaleAfter() time.Duration
}

// BigSegmentsConfigurationFactory is an interface for a factory that creates a BigSegmentsConfiguration.
type BigSegmentsConfigurationFactory interface {
	CreateBigSegmentsConfiguration(context ClientContext) (BigSegmentsConfiguration, error)
}

// BigSegmentStoreFactory is a factory that creates some implementation of BigSegmentStore.
type BigSegmentStoreFactory interface {
	// CreateBigSegmentStore is called by the SDK to create the implementation instance.
	//
	// This happens only when MakeClient or MakeCustomClient is called. The implementation instance
	// is then tied to the life cycle of the LDClient, so it will receive a Close() call when the
	// client is closed.
	//
	// If the factory returns an error, creation of the LDClient fails.
	CreateBigSegmentStore(context ClientContext) (BigSegmentStore, error)
}

// BigSegmentStore is an interface for a read-only data store that allows querying of user
// membership in big segments.
//
// "Big segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
type BigSegmentStore interface {
	io.Closer

	// GetMetadata returns information about the overall state of the store. This method will be
	// called only when the SDK needs the latest state, so it should not be cached.
	GetMetadata() (BigSegmentStoreMetadata, error)

	// GetUserMembership queries the store for a snapshot of the current segment state for a specific
	// user. The userHash is a base64-encoded string produced by hashing the user key as defined by
	// the big segments specification; the store implementation does not need to know the details
	// of how this is done, because it deals only with already-hashed keys, but the string can be
	// assumed to only contain characters that are valid in base64.
	GetUserMembership(userHash string) (BigSegmentMembership, error)
}

// BigSegmentStoreMetadata contains values returned by BigSegmentStore.GetMetadata().
type BigSegmentStoreMetadata struct {
	// LastUpToDate is the timestamp of the last update to the BigSegmentStore. It is zero if
	// the store has never been updated.
	LastUpToDate ldtime.UnixMillisecondTime
}

// BigSegmentMembership is the return type of BigSegmentStore.GetUserMembership(). It is associated
// with a single user, and provides the ability to check whether that user is included in or
// excluded from any number of big segments.
//
// This is an immutable snapshot of the state for this user at the time GetBigSegmentMembership
// was called. Calling CheckMembership should not cause the state to be queried again. The object
// should be safe for concurrent access by multiple goroutines.
type BigSegmentMembership interface {
	// CheckMembership tests whether the user is explicitly included or explicitly excluded in the
	// specified segment, or neither. The segment is identified by a segmentRef which is not the
	// same as the segment key-- it includes the key but also versioning information that the SDK
	// will provide. The store implementation should not be concerned with the format of this.
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
	CheckMembership(segmentRef string) ldvalue.OptionalBool
}

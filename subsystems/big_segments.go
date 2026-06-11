package subsystems

import (
	"io"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// BigSegmentsConfiguration encapsulates the SDK's configuration with regard to Big Segments.
//
// "Big Segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
//
// See ldcomponents.BigSegmentsConfigurationBuilder for more details on these properties.
type BigSegmentsConfiguration interface {
	// GetStore returns the data store instance that is used for Big Segments data.
	GetStore() BigSegmentStore

	// GetContextCacheSize returns the value set by BigSegmentsConfigurationBuilder.CacheSize.
	GetContextCacheSize() int

	// GetContextCacheTime returns the value set by BigSegmentsConfigurationBuilder.CacheTime.
	GetContextCacheTime() time.Duration

	// GetStatusPollInterval returns the value set by BigSegmentsConfigurationBuilder.StatusPollInterval.
	GetStatusPollInterval() time.Duration

	// StaleAfter returns the value set by BigSegmentsConfigurationBuilder.StaleAfter.
	GetStaleAfter() time.Duration
}

// BigSegmentStore is an interface for a read-only data store that allows querying of context
// membership in Big Segments.
//
// "Big Segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
type BigSegmentStore interface {
	io.Closer

	// GetMetadata returns information about the overall state of the store. This method will be
	// called only when the SDK needs the latest state, so it should not be cached.
	GetMetadata() (BigSegmentStoreMetadata, error)

	// GetMembership queries the store for a snapshot of the current segment state for a specific
	// evaluation context. The contextHash is a base64-encoded string produced by hashing the context key
	// as defined by the Big Segments specification; the store implementation does not need to know the
	// details of how this is done, because it deals only with already-hashed keys, but the string can
	// be assumed to only contain characters that are valid in base64.
	GetMembership(contextHash string) (BigSegmentMembership, error)
}

// BigSegmentStoreMetadata contains values returned by BigSegmentStore.GetMetadata().
type BigSegmentStoreMetadata struct {
	// LastUpToDate is the timestamp of the last update to the BigSegmentStore. It is zero if
	// the store has never been updated.
	LastUpToDate ldtime.UnixMillisecondTime
}

// BigSegmentMembership is the return type of BigSegmentStore.GetContextMembership(). It is associated
// with a single evaluation context, and provides the ability to check whether that context is included
// in or excluded from any number of Big Segments.
//
// This is an immutable snapshot of the state for this context at the time GetMembership was
// called. Calling GetMembership should not cause the state to be queried again. The object
// should be safe for concurrent access by multiple goroutines.
type BigSegmentMembership interface {
	// CheckMembership tests whether the context is explicitly included or explicitly excluded in the
	// specified segment, or neither. The segment is identified by a segmentRef which is not the
	// same as the segment key-- it includes the key but also versioning information that the SDK
	// will provide. The store implementation should not be concerned with the format of this.
	//
	// If the context is explicitly included (regardless of whether the context is also explicitly
	// excluded or not-- that is, inclusion takes priority over exclusion), the method returns an
	// OptionalBool with a true value.
	//
	// If the context is explicitly excluded, and is not explicitly included, the method returns an
	// OptionalBool with a false value.
	//
	// If the context's status in the segment is undefined, the method returns OptionalBool{} with no
	// value (so calling IsDefined() on it will return false).
	CheckMembership(segmentRef string) ldvalue.OptionalBool
}

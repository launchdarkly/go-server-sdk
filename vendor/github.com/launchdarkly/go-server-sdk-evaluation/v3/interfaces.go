package evaluation

import (
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// Evaluator is the engine for evaluating feature flags.
type Evaluator interface {
	// Evaluate evaluates a feature flag for the specified context.
	//
	// The flag is passed by reference only for efficiency; the evaluator will never modify any flag
	// properties. Passing a nil flag will result in a panic.
	//
	// The evaluator does not know anything about analytics events; generating any appropriate analytics
	// events is the responsibility of the caller, who can also provide a callback in prerequisiteFlagEventRecorder
	// to be notified if any additional evaluations were done due to prerequisites. The prerequisiteFlagEventRecorder
	// parameter can be nil if you do not need to track prerequisite evaluations.
	Evaluate(
		flag *ldmodel.FeatureFlag,
		context ldcontext.Context,
		prerequisiteFlagEventRecorder PrerequisiteFlagEventRecorder,
	) Result
}

type ArbitraryConfigProvider interface {
	GetArbitraryConfigMapValues(config ldmodel.ArbitraryConfigs, key any) ldvalue.Value
}

// PrerequisiteFlagEventRecorder is a function that Evaluator.Evaluate() will call to record the
// result of a prerequisite flag evaluation.
type PrerequisiteFlagEventRecorder func(PrerequisiteFlagEvent)

// PrerequisiteFlagEvent is the parameter data passed to PrerequisiteFlagEventRecorder.
type PrerequisiteFlagEvent struct {
	// TargetFlagKey is the key of the feature flag that had a prerequisite.
	TargetFlagKey string
	// Context is the context that the flag was evaluated for. We pass this back to the caller, even though the caller
	// already passed it to us in the Evaluate parameters, so that the caller can provide a stateless function for
	// PrerequisiteFlagEventRecorder rather than a closure (since closures are less efficient).
	Context ldcontext.Context
	// PrerequisiteFlag is the full configuration of the prerequisite flag. We need to pass the full flag here rather
	// than just the key because the flag's properties (such as TrackEvents) can affect how events are generated.
	// This is passed by reference for efficiency only, and will never be nil; the PrerequisiteFlagEventRecorder
	// must not modify the flag's properties.
	PrerequisiteFlag *ldmodel.FeatureFlag
	// PrerequisiteResult is the result of evaluating the prerequisite flag.
	PrerequisiteResult Result
	// ExcludeFromSummaries determines if the event will be included in summary information.
	ExcludeFromSummaries bool
}

// DataProvider is an abstraction for querying feature flags and user segments from a data store.
// The caller provides an implementation of this interface to NewEvaluator.
//
// Flags and segments are returned by reference for efficiency only (on the assumption that the
// caller already has these objects in memory); the evaluator will never modify their properties.
type DataProvider interface {
	// GetFeatureFlag attempts to retrieve a feature flag from the data store by key.
	//
	// The evaluator calls this method if a flag contains a prerequisite condition referencing
	// another flag.
	//
	// The method returns nil if the flag was not found. The DataProvider should treat any deleted
	// flag as "not found" even if the data store contains a deleted flag placeholder for it.
	GetFeatureFlag(key string) *ldmodel.FeatureFlag
	// GetSegment attempts to retrieve a user segment from the data store by key.
	//
	// The evaluator calls this method if a clause in a flag rule uses the OperatorSegmentMatch
	// test.
	//
	// The method returns nil if the segment was not found. The DataProvider should treat any deleted
	// segment as "not found" even if the data store contains a deleted segment placeholder for it.
	GetSegment(key string) *ldmodel.Segment

	// GetArbitraryConfigs attempts to retrieve an arbitrary configuration from the data store by key.
	//
	// The evaluator calls this method if a clause in a flag rule uses the OperatorArbitraryConfigsMatch
	// test.
	//
	// The method returns nil if the arbitrary configuration was not found. The DataProvider should treat any deleted
	// arbitrary configuration as "not found" even if the data store contains a deleted arbitrary configuration placeholder for it.
	GetArbitraryConfigs(key string) *ldmodel.ArbitraryConfigs
}

// BigSegmentProvider is an abstraction for querying membership in big segments. The caller
// provides an implementation of this interface to NewEvaluatorWithBigSegments.
type BigSegmentProvider interface {
	// GetMembership queries a snapshot of the current segment state for a specific context
	// key.
	//
	// The underlying big segment store implementation will use a hash of the context key, rather
	// than the raw key. But computing the hash is the responsibility of the BigSegmentProvider
	// implementation rather than the evaluator, because there may already have a cached result for
	// that user, and we don't want to have to compute a hash repeatedly just to query a cache.
	//
	// Any given big segment is specific to one context kind, so we do not specify a context kind
	// here; it is OK for the membership results to include different context kinds for the same
	// key. That is, if for instance the context {kind: "user", key: "x"} is included in big segment
	// S1, and the context {kind: "org", key: "x"} is included in big segment S2, then the query
	// result for key "x" will show that it is included in both S1 and S2; even though those "x"
	// keys are really for two unrelated context kinds, we will always know which kind we mean if
	// we are specifically checking either S1 or S2, because S1 is defined as only applying to the
	// "user" kind and S2 is defined as only applying to the "org" kind.
	//
	// If the returned BigSegmentMembership is nil, it is treated the same as an implementation
	// whose CheckMembership method always returns an empty value.
	GetMembership(
		contextKey string,
	) (BigSegmentMembership, ldreason.BigSegmentsStatus)
}

// BigSegmentMembership is the return type of BigSegmentProvider.GetContextMembership(). It is
// associated with a single context kind and context key, and provides the ability to check whether
// that context is included in or excluded from any number of big segments.
//
// This is an immutable snapshot of the state for this context at the time GetBigSegmentMembership
// was called. Calling CheckMembership should not cause the state to be queried again. The object
// should be safe for concurrent access by multiple goroutines.
//
// This interface also exists in go-server-sdk because it is exposed as part of the public SDK API;
// users can write their own implementations of SDK components, but we do not want application code
// to reference go-server-sdk-evaluation symbols directly as part of that, because this library is
// versioned separately from the SDK. Currently the two interfaces are identical, but it might be
// that the go-server-sdk-evaluation version would diverge from the go-server-sdk version due to
// some internal requirements that aren't relevant to users, in which case go-server-sdk would be
// responsible for bridging the difference.
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

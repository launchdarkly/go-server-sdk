package evaluation

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

// Evaluator is the engine for evaluating feature flags.
type Evaluator interface {
	// Evaluate evaluates a feature flag for the specified user.
	//
	// The prerequisiteFlagEventRecorder parameter can be nil if you do not need to track
	// prerequisite evaluations.
	Evaluate(
		flag ldmodel.FeatureFlag,
		user lduser.User,
		prerequisiteFlagEventRecorder PrerequisiteFlagEventRecorder,
	) ldreason.EvaluationDetail
}

// PrerequisiteFlagEventRecorder is a function that Evaluator.Evaluate() will call to record the
// result of a prerequisite flag evaluation.
type PrerequisiteFlagEventRecorder func(PrerequisiteFlagEvent)

// PrerequisiteFlagEvent is the parameter data passed to PrerequisiteFlagEventRecorder.
type PrerequisiteFlagEvent struct {
	// TargetFlagKey is the key of the feature flag that had a prerequisite.
	TargetFlagKey string
	// User is the user that the flag was evaluated for. We pass this back to the caller, even though the caller
	// already passed it to us in the Evaluate parameters, so that the caller can provide a stateless function for
	// PrerequisiteFlagEventRecorder rather than a closure (since closures are less efficient).
	User lduser.User
	// PrerequisiteFlag is the full configuration of the prerequisite flag. We need to pass the full flag here rather
	// than just the key because the flag's properties (such as TrackEvents) can affect how events are generated.
	PrerequisiteFlag ldmodel.FeatureFlag
	// PrerequisiteResult is the result of evaluating the prerequisite flag.
	PrerequisiteResult ldreason.EvaluationDetail
}

// DataProvider is an abstraction for querying feature flags and user segments from a data store.
// The caller provides an implementation of this interface to NewEvaluator.
type DataProvider interface {
	// GetFeatureFlag attempts to retrieve a feature flag from the data store by key.
	//
	// The evaluator calls this method if a flag contains a prerequisite condition referencing
	// another flag.
	//
	// The second return value is true if successful, false if the flag was not found. The
	// DataProvider should treat any deleted flag as "not found" even if the data store contains
	// a deleted flag placeholder for it. If the second return value is false, the first return
	// value is ignored and can be an empty FeatureFlag{}.
	GetFeatureFlag(key string) (ldmodel.FeatureFlag, bool)
	// GetSegment attempts to retrieve a user segment from the data store by key.
	//
	// The evaluator calls this method if a clause in a flag rule uses the OperatorSegmentMatch
	// test.
	//
	// The second return value is true if successful, false if the segment was not found. The
	// DataProvider should treat any deleted segment as "not found" even if the data store contains
	// a deleted segment placeholder for it. If the second return value is false, the first return
	// value is ignored and can be an empty Segment{}.
	GetSegment(key string) (ldmodel.Segment, bool)
}

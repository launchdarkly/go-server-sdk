package evaluation

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

// FlagEventProperties provides an abstraction for querying flag properties that affect analytics events.
//
// This conforms to the FlagEventProperties interface in the go-sdk-events package, allowing that package to be
// independent from implementation details of the server-side data model
type FlagEventProperties ldmodel.FeatureFlag

// GetKey returns the flag key.
func (p FlagEventProperties) GetKey() string {
	return p.Key
}

// GetVersion returns the flag version.
func (p FlagEventProperties) GetVersion() int {
	return p.Version
}

// IsFullEventTrackingEnabled returns true if the flag has been configured to always generate detailed event data.
func (p FlagEventProperties) IsFullEventTrackingEnabled() bool {
	return p.TrackEvents
}

// GetDebugEventsUntilDate returns zero normally, but if event debugging has been temporarily enabled for the flag,
// it returns the time at which debugging mode should expire.
func (p FlagEventProperties) GetDebugEventsUntilDate() ldtime.UnixMillisecondTime {
	if p.DebugEventsUntilDate == nil {
		return 0
	}
	return *p.DebugEventsUntilDate
}

// IsExperimentationEnabled returns true if, based on the EvaluationReason returned by the flag evaluation, an event for
// that evaluation should have full tracking enabled and always report the reason even if the application didn't
// explicitly request this. For instance, this is true if a rule was matched that had tracking enabled for that specific
// rule.
//
// This differs from IsFullEventTrackingEnabled() in that it is dependent on the result of a specific evaluation; also,
// IsFullEventTrackingEnabled() being true does not imply that the event should always contain a reason, whereas
// IsExperimentationEnabled() being true does force the reason to be included.
func (p FlagEventProperties) IsExperimentationEnabled(reason ldreason.EvaluationReason) bool {
	switch reason.GetKind() {
	case ldreason.EvalReasonFallthrough:
		return p.TrackEventsFallthrough
	case ldreason.EvalReasonRuleMatch:
		i := reason.GetRuleIndex()
		if i >= 0 && i < len(p.Rules) {
			return p.Rules[i].TrackEvents
		}
	}
	return false
}

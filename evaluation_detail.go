package ldclient

// EvalReasonKind defines the possible values of the Kind property of EvaluationReason.
type EvalReasonKind string

const (
	// EvalReasonOff indicates that the flag was off and therefore returned its configured off value.
	EvalReasonOff EvalReasonKind = "OFF"
	// EvalReasonTargetMatch indicates that the user key was specifically targeted for this flag.
	EvalReasonTargetMatch EvalReasonKind = "TARGET_MATCH"
	// EvalReasonRuleMatch indicates that the user matched one of the flag's rules. The RuleIndex
	// and RuleID properties will be set.
	EvalReasonRuleMatch EvalReasonKind = "RULE_MATCH"
	// EvalReasonPrerequisitesFailed indicates that the flag was considered off because it had at
	// least one prerequisite flag that either was off or did not return the desired variation.
	// The PrerequisiteKeys property will be set.
	EvalReasonPrerequisitesFailed EvalReasonKind = "PREREQUISITES_FAILED"
	// EvalReasonFallthrough indicates that the flag was on but the user did not match any targets
	// or rules.
	EvalReasonFallthrough EvalReasonKind = "FALLTHROUGH"
	// EvalReasonError indicates that the flag could not be evaluated, e.g. because it does not
	// exist or due to an unexpected error. In this case the result value will be the default value
	// that the caller passed to the client. The ErrorKind property will be set.
	EvalReasonError EvalReasonKind = "ERROR"
)

// EvalErrorKind defines the possible values of the ErrorKind property of EvaluationReason.
type EvalErrorKind string

const (
	// EvalErrorClientNotReady indicates that the caller tried to evaluate a flag before the client
	// had successfully initialized.
	EvalErrorClientNotReady EvalErrorKind = "CLIENT_NOT_READY"
	// EvalErrorFlagNotFound indicates that the caller provided a flag key that did not match any
	// known flag.
	EvalErrorFlagNotFound EvalErrorKind = "FLAG_NOT_FOUND"
	// EvalErrorMalformedFlag indicates that there was an internal inconsistency in the flag data,
	// e.g. a rule specified a nonexistent variation.
	EvalErrorMalformedFlag EvalErrorKind = "MALFORMED_FLAG"
	// EvalErrorUserNotSpecified indicates that the caller passed a user without a key for the user
	// parameter.
	EvalErrorUserNotSpecified EvalErrorKind = "USER_NOT_SPECIFIED"
	// EvalErrorWrongType indicates that the result value was not of the requested type, e.g. you
	// called BoolVariationDetail but the value was an integer.
	EvalErrorWrongType EvalErrorKind = "WRONG_TYPE"
	// EvalErrorException indicates that an unexpected error stopped flag evaluation; check the
	// log for details.
	EvalErrorException EvalErrorKind = "EXCEPTION"
)

// EvaluationReason describes the reason that a flag evaluation producted a particular value.
type EvaluationReason struct {
	// Kind describes the general category of the reason.
	Kind EvalReasonKind `json:"kind"`
	// ErrorKind describes the type of error, if Kind is equal to EvalReasonError, or nil otherwise.
	ErrorKind *EvalErrorKind `json:"errorKind,omitempty"`
	// RuleIndex is the index of the rule that was matched (0 being the first), if Kind is equal to
	// EvalReasonRuleMatch, or nil otherwise.
	RuleIndex *int `json:"ruleIndex,omitempty"`
	// RuleID is the unique identifier of the rule that was matched, if Kind is equal to EvalReasonRuleMatch,
	// or nil otherwise.
	RuleID *string `json:"ruleId,omitempty"`
	// PrerequisiteKeys are the flag keys of prerequisites that failed, if Kind is equal to
	// EvalReasonPrerequisitesFailed.
	PrerequisiteKeys *[]string `json:"prerequisiteKeys,omitempty"`
}

// EvaluationDetail is an object returned by LDClient.VariationDetail, combining the result of a
// flag evaluation with an explanation of how it was calculated.
type EvaluationDetail struct {
	// Value is the result of the flag evaluation. This will be either one of the flag's variations or
	// the default value that was passed to the Variation method.
	Value interface{}
	// VariationIndex is the index of the returned value within the flag's list of variations, e.g.
	// 0 for the first variation - or nil if the default value was returned.
	VariationIndex *int
	// Reason is an EvaluationReason object describing the main factor that influenced the flag
	// evaluation value.
	Reason EvaluationReason
}

// Explanation is an obsolete type that is used by the deprecated EvaluateExplain method.
//
// Deprecated: Use the VariationDetail methods and the EvaluationDetail type instead.
type Explanation struct {
	Kind                string `json:"kind" bson:"kind"`
	*Target             `json:"target,omitempty"`
	*Rule               `json:"rule,omitempty"`
	*Prerequisite       `json:"prerequisite,omitempty"`
	*VariationOrRollout `json:"fallthrough,omitempty"`
}

func errorReason(kind EvalErrorKind) EvaluationReason {
	return EvaluationReason{Kind: EvalReasonError, ErrorKind: &kind}
}

// Convert the current EvaluationReason struct to the deprecated type used by EvaluateExplain,
// which includes pointers to objects within the flag data model.
func explanationFromEvaluationReason(reason EvaluationReason, flag FeatureFlag, user User) Explanation {
	var ret Explanation
	if reason.Kind == EvalReasonTargetMatch {
		ret.Kind = "target"
	FindTarget:
		for _, target := range flag.Targets {
			for _, value := range target.Values {
				if value == *user.Key {
					ret.Target = &target
					break FindTarget
				}
			}
		}
	} else if reason.Kind == EvalReasonRuleMatch {
		ret.Kind = "rule"
		if reason.RuleIndex != nil && *reason.RuleIndex < len(flag.Rules) {
			rule := flag.Rules[*reason.RuleIndex]
			ret.Rule = &rule
		}
	} else if reason.Kind == EvalReasonPrerequisitesFailed {
		ret.Kind = "prerequisite"
		if reason.PrerequisiteKeys != nil && len(*reason.PrerequisiteKeys) > 0 {
			prereqKey := (*reason.PrerequisiteKeys)[0]
			for _, prereq := range flag.Prerequisites {
				if prereq.Key == prereqKey {
					ret.Prerequisite = &prereq
					break
				}
			}
		}
	} else if reason.Kind == EvalReasonFallthrough {
		ret.Kind = "fallthrough"
		ret.VariationOrRollout = &flag.Fallthrough
	} else if reason.Kind == EvalReasonError {
		// This isn't actually possible with EvaluateExplain
		ret.Kind = "error"
	}
	return ret
}

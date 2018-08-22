package ldclient

import (
	"fmt"
	"strings"
)

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
// Specific kinds of reasons have their own types that implement this interface.
type EvaluationReason interface {
	// GetKind describes the general category of the reason.
	GetKind() EvalReasonKind
}

type evaluationReasonBase struct {
	// Kind describes the general category of the reason.
	Kind EvalReasonKind `json:"kind"`
}

func (r evaluationReasonBase) GetKind() EvalReasonKind {
	return r.Kind
}

// EvaluationReasonOff means that the flag was off and therefore returned its configured off value.
type EvaluationReasonOff struct {
	evaluationReasonBase
}

var evalReasonOffInstance = EvaluationReasonOff{
	evaluationReasonBase: evaluationReasonBase{Kind: EvalReasonOff},
}

func (r EvaluationReasonOff) String() string {
	return string(r.GetKind())
}

// EvaluationReasonTargetMatch means that the user key was specifically targeted for this flag.
type EvaluationReasonTargetMatch struct {
	evaluationReasonBase
}

var evalReasonTargetMatchInstance = EvaluationReasonTargetMatch{
	evaluationReasonBase: evaluationReasonBase{Kind: EvalReasonTargetMatch},
}

func (r EvaluationReasonTargetMatch) String() string {
	return string(r.GetKind())
}

// EvaluationReasonRuleMatch means that the user matched one of the flag's rules.
type EvaluationReasonRuleMatch struct {
	evaluationReasonBase
	// RuleIndex is the index of the rule that was matched (0 being the first).
	RuleIndex int `json:"ruleIndex"`
	// RuleID is the unique identifier of the rule that was matched.
	RuleID string `json:"ruleId"`
}

func newEvalReasonRuleMatch(ruleIndex int, ruleID string) EvaluationReasonRuleMatch {
	return EvaluationReasonRuleMatch{
		evaluationReasonBase: evaluationReasonBase{Kind: EvalReasonRuleMatch},
		RuleIndex:            ruleIndex,
		RuleID:               ruleID,
	}
}

func (r EvaluationReasonRuleMatch) String() string {
	return fmt.Sprintf("%s(%d,%s)", r.GetKind(), r.RuleIndex, r.RuleID)
}

// EvaluationReasonPrerequisitesFailed means that the flag was considered off because it had at
// least one prerequisite flag that either was off or did not return the desired variation.
type EvaluationReasonPrerequisitesFailed struct {
	evaluationReasonBase
	// PrerequisiteKeys are the flag keys of the prerequisites that failed.
	PrerequisiteKeys []string `json:"prerequisiteKeys"`
}

func newEvalReasonPrerequisitesFailed(prereqKeys []string) EvaluationReasonPrerequisitesFailed {
	return EvaluationReasonPrerequisitesFailed{
		evaluationReasonBase: evaluationReasonBase{Kind: EvalReasonPrerequisitesFailed},
		PrerequisiteKeys:     prereqKeys,
	}
}

func (r EvaluationReasonPrerequisitesFailed) String() string {
	return fmt.Sprintf("%s(%s)", r.GetKind(), strings.Join(r.PrerequisiteKeys, ","))
}

// EvaluationReasonFallthrough means that the flag was on but the user did not match any targets
// or rules.
type EvaluationReasonFallthrough struct {
	evaluationReasonBase
}

var evalReasonFallthroughInstance = EvaluationReasonFallthrough{
	evaluationReasonBase: evaluationReasonBase{Kind: EvalReasonFallthrough},
}

func (r EvaluationReasonFallthrough) String() string {
	return string(r.GetKind())
}

// EvaluationReasonError means that the flag could not be evaluated, e.g. because it does not
// exist or due to an unexpected error.
type EvaluationReasonError struct {
	evaluationReasonBase
	// ErrorKind describes the type of error.
	ErrorKind EvalErrorKind `json:"errorKind"`
}

func newEvalReasonError(kind EvalErrorKind) EvaluationReasonError {
	return EvaluationReasonError{
		evaluationReasonBase: evaluationReasonBase{Kind: EvalReasonError},
		ErrorKind:            kind,
	}
}

func (r EvaluationReasonError) String() string {
	return fmt.Sprintf("%s(%s)", r.GetKind(), r.ErrorKind)
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

// Convert the current EvaluationReason struct to the deprecated type used by EvaluateExplain,
// which includes pointers to objects within the flag data model.
func explanationFromEvaluationReason(reason EvaluationReason, flag FeatureFlag, user User) Explanation {
	var ret Explanation
	switch r := reason.(type) {
	case EvaluationReasonTargetMatch:
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
	case EvaluationReasonRuleMatch:
		ret.Kind = "rule"
		if r.RuleIndex < len(flag.Rules) {
			rule := flag.Rules[r.RuleIndex]
			ret.Rule = &rule
		}
	case EvaluationReasonPrerequisitesFailed:
		ret.Kind = "prerequisite"
		if len(r.PrerequisiteKeys) > 0 {
			prereqKey := r.PrerequisiteKeys[0]
			for _, prereq := range flag.Prerequisites {
				if prereq.Key == prereqKey {
					ret.Prerequisite = &prereq
					break
				}
			}
		}
	case EvaluationReasonFallthrough:
		ret.Kind = "fallthrough"
		ret.VariationOrRollout = &flag.Fallthrough
	case EvaluationReasonError:
		// This isn't actually possible with EvaluateExplain
		ret.Kind = "error"
	}
	return ret
}

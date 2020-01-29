package ldclient

import (
	"encoding/json"
	"fmt"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// EvalReasonKind defines the possible values of the Kind property of EvaluationReason.
type EvalReasonKind string

const (
	// EvalReasonOff indicates that the flag was off and therefore returned its configured off value.
	EvalReasonOff EvalReasonKind = "OFF"
	// EvalReasonTargetMatch indicates that the user key was specifically targeted for this flag.
	EvalReasonTargetMatch EvalReasonKind = "TARGET_MATCH"
	// EvalReasonRuleMatch indicates that the user matched one of the flag's rules.
	EvalReasonRuleMatch EvalReasonKind = "RULE_MATCH"
	// EvalReasonPrerequisiteFailed indicates that the flag was considered off because it had at
	// least one prerequisite flag that either was off or did not return the desired variation.
	EvalReasonPrerequisiteFailed EvalReasonKind = "PREREQUISITE_FAILED"
	// EvalReasonFallthrough indicates that the flag was on but the user did not match any targets
	// or rules.
	EvalReasonFallthrough EvalReasonKind = "FALLTHROUGH"
	// EvalReasonError indicates that the flag could not be evaluated, e.g. because it does not
	// exist or due to an unexpected error. In this case the result value will be the default value
	// that the caller passed to the client.
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
	kind            EvalReasonKind
	ruleIndex       int
	ruleID          string
	prerequisiteKey string
	errorKind       EvalErrorKind
}

// String returns a concise string representation of the reason. Examples: "OFF", "ERROR(WRONG_TYPE)".
func (r EvaluationReason) String() string {
	switch r.kind {
	case EvalReasonRuleMatch:
		return fmt.Sprintf("%s(%d,%s)", r.kind, r.ruleIndex, r.ruleID)
	case EvalReasonPrerequisiteFailed:
		return fmt.Sprintf("%s(%s)", r.kind, r.prerequisiteKey)
	case EvalReasonError:
		return fmt.Sprintf("%s(%s)", r.kind, r.errorKind)
	default:
		return string(r.GetKind())
	}
}

// GetKind describes the general category of the reason.
func (r EvaluationReason) GetKind() EvalReasonKind {
	return r.kind
}

// GetRuleIndex provides the index of the rule that was matched (0 being the first), if
// the Kind is EvalReasonRuleMatch. Otherwise it returns -1.
func (r EvaluationReason) GetRuleIndex() int {
	if r.kind == EvalReasonRuleMatch {
		return r.ruleIndex
	}
	return -1
}

// GetRuleID provides the unique identifier of the rule that was matched, if the Kind is
// EvalReasonRuleMatch. Otherwise it returns an empty string. Unlike the rule index, this
// identifier will not change if other rules are added or deleted.
func (r EvaluationReason) GetRuleID() string {
	return r.ruleID
}

// GetPrerequisiteKey provides the flag key of the prerequisite that failed, if the Kind
// is EvalReasonPrerequisiteFailed. Otherwise it returns an empty string.
func (r EvaluationReason) GetPrerequisiteKey() string {
	return r.prerequisiteKey
}

// GetErrorKind describes the general category of the error, if the Kind is EvalReasonError.
// Otherwise it returns an empty string.
func (r EvaluationReason) GetErrorKind() EvalErrorKind {
	return r.errorKind
}

func newEvalReasonOff() EvaluationReason {
	return EvaluationReason{kind: EvalReasonOff}
}

func newEvalReasonFallthrough() EvaluationReason {
	return EvaluationReason{kind: EvalReasonFallthrough}
}

func newEvalReasonTargetMatch() EvaluationReason {
	return EvaluationReason{kind: EvalReasonTargetMatch}
}

func newEvalReasonRuleMatch(ruleIndex int, ruleID string) EvaluationReason {
	return EvaluationReason{kind: EvalReasonRuleMatch, ruleIndex: ruleIndex, ruleID: ruleID}
}

func newEvalReasonPrerequisiteFailed(prereqKey string) EvaluationReason {
	return EvaluationReason{kind: EvalReasonPrerequisiteFailed, prerequisiteKey: prereqKey}
}

func newEvalReasonError(errorKind EvalErrorKind) EvaluationReason {
	return EvaluationReason{kind: EvalReasonError, errorKind: errorKind}
}

// EvaluationDetail is an object returned by LDClient.VariationDetail, combining the result of a
// flag evaluation with an explanation of how it was calculated.
type EvaluationDetail struct {
	// Value is the result of the flag evaluation. This will be either one of the flag's variations or
	// the default value that was passed to the Variation method.
	//
	// Deprecated: Use JSONValue instead. The Value property will be removed in a future version.
	Value interface{}
	// JSONValue is the result of the flag evaluation, represented with the ldvalue.Value type.
	// This is always the same value you would get by calling LDClient.JSONVariation(). You can
	// convert it to a bool, int, string, etc. using methods of ldvalue.Value.
	//
	// This property is preferred over EvaluationDetail.Value, because the interface{} type of Value
	// can expose a mutable data structure (slice or map) and accidentally modifying such a structure
	// could affect SDK behavior.
	JSONValue ldvalue.Value
	// VariationIndex is the index of the returned value within the flag's list of variations, e.g.
	// 0 for the first variation - or nil if the default value was returned.
	VariationIndex *int
	// Reason is an EvaluationReason object describing the main factor that influenced the flag
	// evaluation value.
	Reason EvaluationReason
}

// NewEvaluationDetail creates an EvaluationDetail, specifying all fields. The deprecated Value property is set
// to the same value that is wrapped by jsonValue.
func NewEvaluationDetail(jsonValue ldvalue.Value, variationIndex *int, reason EvaluationReason) EvaluationDetail {
	return EvaluationDetail{
		Value:          jsonValue.UnsafeArbitraryValue(), //nolint (using deprecated method)
		JSONValue:      jsonValue,
		VariationIndex: variationIndex,
		Reason:         reason,
	}
}

// NewEvaluationError creates an EvaluationDetail describing an error. The deprecated Value property is set
// to the same value that is wrapped by jsonValue.
func NewEvaluationError(jsonValue ldvalue.Value, errorKind EvalErrorKind) EvaluationDetail {
	return EvaluationDetail{
		Value:     jsonValue.UnsafeArbitraryValue(), //nolint (using deprecated method)
		JSONValue: jsonValue,
		Reason:    newEvalReasonError(errorKind),
	}
}

// IsDefaultValue returns true if the result of the evaluation was the default value.
func (d EvaluationDetail) IsDefaultValue() bool {
	return d.VariationIndex == nil
}

type evaluationReasonForMarshaling struct {
	Kind            EvalReasonKind `json:"kind"`
	RuleIndex       *int           `json:"ruleIndex,omitempty"`
	RuleID          string         `json:"ruleId,omitempty"`
	PrerequisiteKey string         `json:"prerequisiteKey,omitempty"`
	ErrorKind       EvalErrorKind  `json:"errorKind,omitempty"`
}

// MarshalJSON implements custom JSON serialization for EvaluationReason.
func (r EvaluationReason) MarshalJSON() ([]byte, error) {
	if r.kind == "" {
		return []byte("null"), nil
	}
	erm := evaluationReasonForMarshaling{
		Kind:            r.kind,
		RuleID:          r.ruleID,
		PrerequisiteKey: r.prerequisiteKey,
		ErrorKind:       r.errorKind,
	}
	if r.kind == EvalReasonRuleMatch {
		erm.RuleIndex = &r.ruleIndex
	}
	return json.Marshal(erm)
}

// UnmarshalJSON implements custom JSON deserialization for EvaluationReason.
func (r *EvaluationReason) UnmarshalJSON(data []byte) error {
	var erm evaluationReasonForMarshaling
	if err := json.Unmarshal(data, &erm); err != nil {
		return nil
	}
	*r = EvaluationReason{
		kind:            erm.Kind,
		ruleID:          erm.RuleID,
		prerequisiteKey: erm.PrerequisiteKey,
		errorKind:       erm.ErrorKind,
	}
	if erm.RuleIndex != nil {
		r.ruleIndex = *erm.RuleIndex
	}
	return nil
}

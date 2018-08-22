package ldclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOffReasonSerialization(t *testing.T) {
	reason := EvaluationReason{Kind: EvalReasonOff}
	expected := `{"kind":"OFF"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
}

func TestTargetMatchReasonSerialization(t *testing.T) {
	reason := EvaluationReason{Kind: EvalReasonTargetMatch}
	expected := `{"kind":"TARGET_MATCH"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
}

func TestRuleMatchReasonSerialization(t *testing.T) {
	reason := EvaluationReason{Kind: EvalReasonRuleMatch, RuleIndex: intPtr(1), RuleID: strPtr("id")}
	expected := `{"kind":"RULE_MATCH","ruleIndex":1,"ruleId":"id"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
}

func TestPrerequisitesFailedReasonSerialization(t *testing.T) {
	reason := EvaluationReason{Kind: EvalReasonPrerequisitesFailed, PrerequisiteKeys: &[]string{"key1", "key2"}}
	expected := `{"kind":"PREREQUISITES_FAILED","prerequisiteKeys":["key1", "key2"]}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
}

func TestFallthroughReasonSerialization(t *testing.T) {
	reason := EvaluationReason{Kind: EvalReasonFallthrough}
	expected := `{"kind":"FALLTHROUGH"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
}

func TestErrorReasonSerialization(t *testing.T) {
	reason := errorReason(EvalErrorException)
	expected := `{"kind":"ERROR","errorKind":"EXCEPTION"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
}

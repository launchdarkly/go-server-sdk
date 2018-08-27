package ldclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOffReasonSerialization(t *testing.T) {
	reason := evalReasonOffInstance
	expected := `{"kind":"OFF"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "OFF", reason.String())

	var r1 EvaluationReasonContainer
	err = json.Unmarshal(actual, &r1)
	assert.NoError(t, err)
	assert.Equal(t, reason, r1.Reason)
}

func TestTargetMatchReasonSerialization(t *testing.T) {
	reason := evalReasonTargetMatchInstance
	expected := `{"kind":"TARGET_MATCH"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "TARGET_MATCH", reason.String())

	var r1 EvaluationReasonContainer
	err = json.Unmarshal(actual, &r1)
	assert.NoError(t, err)
	assert.Equal(t, reason, r1.Reason)
}

func TestRuleMatchReasonSerialization(t *testing.T) {
	reason := newEvalReasonRuleMatch(1, "id")
	expected := `{"kind":"RULE_MATCH","ruleIndex":1,"ruleId":"id"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "RULE_MATCH(1,id)", reason.String())

	var r1 EvaluationReasonContainer
	err = json.Unmarshal(actual, &r1)
	assert.NoError(t, err)
	assert.Equal(t, reason, r1.Reason)
}

func TestPrerequisiteFailedReasonSerialization(t *testing.T) {
	reason := newEvalReasonPrerequisiteFailed("key")
	expected := `{"kind":"PREREQUISITE_FAILED","prerequisiteKey":"key"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "PREREQUISITE_FAILED(key)", reason.String())

	var r1 EvaluationReasonContainer
	err = json.Unmarshal(actual, &r1)
	assert.NoError(t, err)
	assert.Equal(t, reason, r1.Reason)
}

func TestFallthroughReasonSerialization(t *testing.T) {
	reason := evalReasonFallthroughInstance
	expected := `{"kind":"FALLTHROUGH"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "FALLTHROUGH", reason.String())

	var r1 EvaluationReasonContainer
	err = json.Unmarshal(actual, &r1)
	assert.NoError(t, err)
	assert.Equal(t, reason, r1.Reason)
}

func TestErrorReasonSerialization(t *testing.T) {
	reason := newEvalReasonError(EvalErrorException)
	expected := `{"kind":"ERROR","errorKind":"EXCEPTION"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "ERROR(EXCEPTION)", reason.String())

	var r1 EvaluationReasonContainer
	err = json.Unmarshal(actual, &r1)
	assert.NoError(t, err)
	assert.Equal(t, reason, r1.Reason)
}

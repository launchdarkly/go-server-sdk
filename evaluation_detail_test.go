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
}

func TestTargetMatchReasonSerialization(t *testing.T) {
	reason := evalReasonTargetMatchInstance
	expected := `{"kind":"TARGET_MATCH"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "TARGET_MATCH", reason.String())
}

func TestRuleMatchReasonSerialization(t *testing.T) {
	reason := newEvalReasonRuleMatch(1, "id")
	expected := `{"kind":"RULE_MATCH","ruleIndex":1,"ruleId":"id"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "RULE_MATCH(1,id)", reason.String())
}

func TestPrerequisitesFailedReasonSerialization(t *testing.T) {
	reason := newEvalReasonPrerequisitesFailed([]string{"key1", "key2"})
	expected := `{"kind":"PREREQUISITES_FAILED","prerequisiteKeys":["key1", "key2"]}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "PREREQUISITES_FAILED(key1,key2)", reason.String())
}

func TestFallthroughReasonSerialization(t *testing.T) {
	reason := evalReasonFallthroughInstance
	expected := `{"kind":"FALLTHROUGH"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "FALLTHROUGH", reason.String())
}

func TestErrorReasonSerialization(t *testing.T) {
	reason := newEvalReasonError(EvalErrorException)
	expected := `{"kind":"ERROR","errorKind":"EXCEPTION"}`
	actual, err := json.Marshal(reason)
	assert.NoError(t, err)
	assert.JSONEq(t, expected, string(actual))
	assert.Equal(t, "ERROR(EXCEPTION)", reason.String())
}

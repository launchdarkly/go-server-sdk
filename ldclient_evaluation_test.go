package ldclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

var evalTestUser = NewUser("userkey")
var evalTestUserWithNilKey = User{Name: strPtr("Bob")}

func makeTestFlag(key string, fallThroughVariation int, variations ...interface{}) *FeatureFlag {
	return &FeatureFlag{
		Key:         key,
		Version:     1,
		On:          true,
		Fallthrough: VariationOrRollout{Variation: &fallThroughVariation},
		Variations:  variations,
	}
}

func assertEvalEvent(t *testing.T, client *LDClient, flag *FeatureFlag, user User, value interface{}, variation int, defaultVal interface{}, reason *EvaluationReason) {
	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(FeatureRequestEvent)
	expectedEvent := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e.CreationDate,
			User:         user,
		},
		Key:       flag.Key,
		Version:   &flag.Version,
		Value:     value,
		Variation: intPtr(variation),
		Default:   defaultVal,
		Reason:    reason,
	}
	assert.Equal(t, expectedEvent, e)
}

func TestBoolVariation(t *testing.T) {
	expected := true
	defaultVal := false
	flag := makeTestFlag("validFeatureKey", 1, false, true)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.BoolVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, nil)
}

func TestBoolVariationDetail(t *testing.T) {
	expected := true
	defaultVal := false
	flag := makeTestFlag("validFeatureKey", 1, false, true)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.BoolVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value)
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, EvalReasonFallthrough, detail.Reason.Kind)

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, &detail.Reason)
}

func TestIntVariation(t *testing.T) {
	expected := 100
	defaultVal := 10000
	flag := makeTestFlag("validFeatureKey", 1, float64(-1), float64(expected))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.IntVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, int(expected), actual)

	assertEvalEvent(t, client, flag, evalTestUser, float64(expected), 1, float64(defaultVal), nil)
}

func TestIntVariationDetail(t *testing.T) {
	expected := 100
	defaultVal := 10000
	flag := makeTestFlag("validFeatureKey", 1, float64(-1), float64(expected))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.IntVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, float64(expected), detail.Value)
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, EvalReasonFallthrough, detail.Reason.Kind)

	assertEvalEvent(t, client, flag, evalTestUser, float64(expected), 1, float64(defaultVal), &detail.Reason)
}

func TestFloat64Variation(t *testing.T) {
	expected := 100.01
	defaultVal := 0.0
	flag := makeTestFlag("validFeatureKey", 1, -1.0, expected)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.Float64Variation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, nil)
}

func TestFloat64VariationDetail(t *testing.T) {
	expected := 100.01
	defaultVal := 0.0
	flag := makeTestFlag("validFeatureKey", 1, -1.0, expected)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.Float64VariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value)
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, EvalReasonFallthrough, detail.Reason.Kind)

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, &detail.Reason)
}

func TestStringVariation(t *testing.T) {
	expected := "b"
	defaultVal := "a"
	flag := makeTestFlag("validFeatureKey", 1, "a", "b")

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.StringVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, nil)
}

func TestStringVariationDetail(t *testing.T) {
	expected := "b"
	defaultVal := "a"
	flag := makeTestFlag("validFeatureKey", 1, "a", "b")

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.StringVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value)
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, EvalReasonFallthrough, detail.Reason.Kind)

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, &detail.Reason)
}

func TestJsonVariation(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	otherValue := map[string]interface{}{"field1": "value1"}
	expectedJSON, _ := json.Marshal(expectedValue)

	flag := makeTestFlag("validFeatureKey", 1, otherValue, expectedValue)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	var actual json.RawMessage
	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, err := client.JsonVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(expectedJSON), actual)

	assertEvalEvent(t, client, flag, evalTestUser, expectedValue, 1, defaultVal, nil)
}

func TestJsonVariationDetail(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	otherValue := map[string]interface{}{"field1": "value1"}
	expectedJSON, _ := json.Marshal(expectedValue)

	flag := makeTestFlag("validFeatureKey", 1, otherValue, expectedValue)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	var actual json.RawMessage
	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, detail, err := client.JsonVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(expectedJSON), actual)
	assert.Equal(t, expectedValue, detail.Value)
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, EvalReasonFallthrough, detail.Reason.Kind)

	assertEvalEvent(t, client, flag, evalTestUser, expectedValue, 1, defaultVal, &detail.Reason)
}

func TestEvaluatingUnknownFlagSendsEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	_, err := client.StringVariation("flagKey", evalTestUser, "x")
	assert.Error(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(FeatureRequestEvent)
	expectedEvent := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e.CreationDate,
			User:         evalTestUser,
		},
		Key:       "flagKey",
		Version:   nil,
		Value:     "x",
		Variation: nil,
		Default:   "x",
		PrereqOf:  nil,
	}
	assert.Equal(t, expectedEvent, e)
}

func TestEvaluatingFlagWithNilUserKeySendsEvent(t *testing.T) {
	flag := makeTestFlag("flagKey", 1, "a", "b")
	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	_, err := client.StringVariation(flag.Key, evalTestUserWithNilKey, "x")
	assert.Error(t, err)

	events := client.eventProcessor.(*testEventProcessor).events

	assert.Equal(t, 1, len(events))
	e := events[0].(FeatureRequestEvent)
	expectedEvent := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e.CreationDate,
			User:         evalTestUserWithNilKey,
		},
		Key:       flag.Key,
		Version:   &flag.Version,
		Value:     "x",
		Variation: nil,
		Default:   "x",
		PrereqOf:  nil,
	}
	assert.Equal(t, expectedEvent, e)
}

func TestEvaluatingFlagWithPrerequisiteSendsPrerequisiteEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag0 := makeTestFlag("flag0", 1, "a", "b")
	flag0.Prerequisites = []Prerequisite{
		Prerequisite{Key: "flag1", Variation: 1},
	}
	flag1 := makeTestFlag("flag1", 1, "c", "d")
	client.store.Upsert(Features, flag0)
	client.store.Upsert(Features, flag1)

	user := NewUser("userKey")
	_, err := client.StringVariation(flag0.Key, user, "x")
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 2, len(events))

	e0 := events[0].(FeatureRequestEvent)
	expected0 := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e0.CreationDate,
			User:         user,
		},
		Key:       flag1.Key,
		Version:   &flag1.Version,
		Value:     "d",
		Variation: intPtr(1),
		Default:   nil,
		PrereqOf:  &flag0.Key,
	}
	assert.Equal(t, expected0, e0)

	e1 := events[1].(FeatureRequestEvent)
	expected1 := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e1.CreationDate,
			User:         user,
		},
		Key:       flag0.Key,
		Version:   &flag0.Version,
		Value:     "b",
		Variation: intPtr(1),
		Default:   "x",
		PrereqOf:  nil,
	}
	assert.Equal(t, expected1, e1)
}

func TestAllFlags(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag0 := makeTestFlag("flag0", 1, "a", "b")
	flag1 := makeTestFlag("flag1", 1, "c", "d")
	client.store.Upsert(Features, flag0)
	client.store.Upsert(Features, flag1)

	result := client.AllFlags(evalTestUser)
	expected := map[string]interface{}{"flag0": "b", "flag1": "d"}
	assert.Equal(t, expected, result)
}

func TestAllFlagsReturnsNilIfUserKeyIsNil(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag0 := makeTestFlag("flag0", 1, "a", "b")
	flag1 := makeTestFlag("flag1", 1, "c", "d")
	client.store.Upsert(Features, flag0)
	client.store.Upsert(Features, flag1)

	result := client.AllFlags(evalTestUserWithNilKey)
	assert.Nil(t, result)
}

func TestAllFlagsStateGetsState(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag1 := FeatureFlag{
		Key:          "key1",
		Version:      100,
		OffVariation: intPtr(0),
		Variations:   []interface{}{"value1"},
	}
	date := uint64(1000)
	flag2 := FeatureFlag{
		Key:                  "key2",
		Version:              200,
		OffVariation:         intPtr(1),
		Variations:           []interface{}{"x", "value2"},
		TrackEvents:          true,
		DebugEventsUntilDate: &date,
	}
	client.store.Upsert(Features, &flag1)
	client.store.Upsert(Features, &flag2)

	state := client.AllFlagsState(NewUser("userkey"))
	assert.True(t, state.IsValid())

	expectedString := `{
		"key1":"value1",
		"key2":"value2",
		"$flagsState":{
	  		"key1":{
				"variation":0,"version":100,"trackEvents":false
			},
			"key2": {
				"variation":1,"version":200,"trackEvents":true,"debugEventsUntilDate":1000
			}
		},
		"$valid":true
	}`
	actualBytes, err := json.Marshal(state)
	assert.NoError(t, err)
	assert.JSONEq(t, expectedString, string(actualBytes))
}

func TestAllFlagsStateReturnsEmptyStateForNilUserKey(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag1 := makeTestFlag("key1", 0, "value1")
	flag2 := makeTestFlag("key2", 0, "value2")
	client.store.Upsert(Features, flag1)
	client.store.Upsert(Features, flag2)

	state := client.AllFlagsState(User{})
	assert.False(t, state.IsValid())
	assert.Nil(t, state.ToValuesMap())
}

func TestAllFlagsStateCanFilterForOnlyClientSideFlags(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag1 := FeatureFlag{Key: "server-side-1"}
	flag2 := FeatureFlag{Key: "server-side-2"}
	flag3 := FeatureFlag{
		Key:          "client-side-1",
		OffVariation: intPtr(0),
		Variations:   []interface{}{"value1"},
		ClientSide:   true,
	}
	flag4 := FeatureFlag{
		Key:          "client-side-2",
		OffVariation: intPtr(0),
		Variations:   []interface{}{"value2"},
		ClientSide:   true,
	}
	client.store.Upsert(Features, &flag1)
	client.store.Upsert(Features, &flag2)
	client.store.Upsert(Features, &flag3)
	client.store.Upsert(Features, &flag4)

	state := client.AllFlagsState(NewUser("userkey"), ClientSideOnly)
	assert.True(t, state.IsValid())

	expectedValues := map[string]interface{}{"client-side-1": "value1", "client-side-2": "value2"}
	assert.Equal(t, expectedValues, state.ToValuesMap())
}

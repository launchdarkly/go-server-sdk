package ldclient

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v1/ldvalue"
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

func makeMalformedFlag(key string) *FeatureFlag {
	return &FeatureFlag{Key: key, On: false, OffVariation: intPtr(-1)}
}

func assertEvalEvent(t *testing.T, client *LDClient, flag *FeatureFlag, user User, value ldvalue.Value,
	variation int, defaultVal ldvalue.Value, reason EvaluationReason) {
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
		Value:     value.UnsafeArbitraryValue(),
		Variation: intPtr(variation),
		Default:   defaultVal.UnsafeArbitraryValue(),
		Reason:    EvaluationReasonContainer{reason},
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

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Bool(expected), 1, ldvalue.Bool(defaultVal), nil)
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
	assert.Equal(t, expected, detail.JSONValue.BoolValue())
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Bool(expected), 1, ldvalue.Bool(defaultVal), detail.Reason)
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

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Int(expected), 1, ldvalue.Int(defaultVal), nil)
}

func TestIntVariationRoundsFloatTowardZero(t *testing.T) {
	flag1 := makeTestFlag("flag1", 1, float64(-1), 2.25)
	flag2 := makeTestFlag("flag2", 1, float64(-1), 2.75)
	flag3 := makeTestFlag("flag3", 1, float64(-1), -2.25)
	flag4 := makeTestFlag("flag4", 1, float64(-1), -2.75)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag1)
	client.store.Upsert(Features, flag2)
	client.store.Upsert(Features, flag3)
	client.store.Upsert(Features, flag4)

	actual, err := client.IntVariation(flag1.Key, evalTestUser, 0)
	assert.NoError(t, err)
	assert.Equal(t, 2, actual)

	actual, err = client.IntVariation(flag2.Key, evalTestUser, 0)
	assert.NoError(t, err)
	assert.Equal(t, 2, actual)

	actual, err = client.IntVariation(flag3.Key, evalTestUser, 0)
	assert.NoError(t, err)
	assert.Equal(t, -2, actual)

	actual, err = client.IntVariation(flag4.Key, evalTestUser, 0)
	assert.NoError(t, err)
	assert.Equal(t, -2, actual)
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
	assert.Equal(t, expected, detail.JSONValue.IntValue())
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Int(expected), 1, ldvalue.Int(defaultVal), detail.Reason)
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

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Float64(expected), 1, ldvalue.Float64(defaultVal), nil)
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
	assert.Equal(t, expected, detail.JSONValue.Float64Value())
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Float64(expected), 1, ldvalue.Float64(defaultVal), detail.Reason)
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

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.String(expected), 1, ldvalue.String(defaultVal), nil)
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
	assert.Equal(t, expected, detail.JSONValue.StringValue())
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.String(expected), 1, ldvalue.String(defaultVal), detail.Reason)
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

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), 1, ldvalue.CopyArbitraryValue(defaultVal), nil)
}

func TestJsonVariationDetail(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	otherValue := map[string]interface{}{"field1": "value1"}
	expectedJSON, _ := json.Marshal(expectedValue)
	expectedRaw := json.RawMessage(expectedJSON)

	flag := makeTestFlag("validFeatureKey", 1, otherValue, expectedValue)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	var actual json.RawMessage
	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, detail, err := client.JsonVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expectedRaw, actual)
	assert.Equal(t, expectedValue, detail.Value)
	assert.Equal(t, expectedRaw, detail.JSONValue.AsRaw())
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), 1, ldvalue.CopyArbitraryValue(defaultVal), detail.Reason)
}

func TestJSONRawVariation(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	otherValue := map[string]interface{}{"field1": "value1"}
	expectedJSON, _ := json.Marshal(expectedValue)

	flag := makeTestFlag("validFeatureKey", 1, otherValue, expectedValue)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, err := client.JSONVariation("validFeatureKey", evalTestUser, ldvalue.Raw(defaultVal))

	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(expectedJSON), actual.AsRaw())

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), 1, ldvalue.CopyArbitraryValue(defaultVal), nil)
}

func TestJSONRawVariationDetail(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	otherValue := map[string]interface{}{"field1": "value1"}
	expectedJSON, _ := json.Marshal(expectedValue)
	expectedRaw := json.RawMessage(expectedJSON)

	flag := makeTestFlag("validFeatureKey", 1, otherValue, expectedValue)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, detail, err := client.JSONVariationDetail("validFeatureKey", evalTestUser, ldvalue.Raw(defaultVal))

	assert.NoError(t, err)
	assert.Equal(t, expectedRaw, actual.AsRaw())
	assert.Equal(t, expectedValue, detail.Value)
	assert.Equal(t, expectedRaw, detail.JSONValue.AsRaw())
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), 1, ldvalue.CopyArbitraryValue(defaultVal), detail.Reason)
}

func TestJSONVariation(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})
	otherValue := ldvalue.CopyArbitraryValue(map[string]interface{}{"field1": "value1"})

	flag := makeTestFlag("validFeatureKey", 1, otherValue.AsArbitraryValue(), expected.AsArbitraryValue())

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := ldvalue.String("no")
	actual, err := client.JSONVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected.AsArbitraryValue(), actual.AsArbitraryValue()) // assert.Equal isn't currently reliable for complex Value types

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, nil)
}

func TestJSONVariationDetail(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})
	otherValue := ldvalue.CopyArbitraryValue(map[string]interface{}{"field1": "value1"})

	flag := makeTestFlag("validFeatureKey", 1, otherValue.AsArbitraryValue(), expected.AsArbitraryValue())

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := ldvalue.String("no")
	actual, detail, err := client.JSONVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected.AsArbitraryValue(), actual.AsArbitraryValue())
	assert.Equal(t, expected.AsArbitraryValue(), detail.Value)
	assert.Equal(t, expected.AsArbitraryValue(), detail.JSONValue.AsArbitraryValue())
	assert.Equal(t, intPtr(1), detail.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, detail.Reason)
}

func TestEvaluatingUnknownFlagReturnsDefault(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	value, err := client.StringVariation("flagKey", evalTestUser, "default")
	assert.Error(t, err)
	assert.Equal(t, "default", value)
}

func TestEvaluatingUnknownFlagReturnsDefaultWithDetail(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	_, detail, err := client.StringVariationDetail("flagKey", evalTestUser, "default")
	assert.Error(t, err)
	assert.Equal(t, "default", detail.Value)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, newEvalReasonError(EvalErrorFlagNotFound), detail.Reason)
	assert.True(t, detail.IsDefaultValue())
}

func TestDefaultIsReturnedIfFlagEvaluatesToNil(t *testing.T) {
	flag := FeatureFlag{
		Key:          "flagKey",
		On:           false,
		OffVariation: nil,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	value, err := client.StringVariation("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "default", value)
}

func TestDefaultIsReturnedIfFlagEvaluatesToNilWithDetail(t *testing.T) {
	flag := FeatureFlag{
		Key:          "flagKey",
		On:           false,
		OffVariation: nil,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	_, detail, err := client.StringVariationDetail("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "default", detail.Value)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, evalReasonOffInstance, detail.Reason)
}

func TestEventTrackingAndReasonCanBeForcedForRule(t *testing.T) {
	flag := FeatureFlag{
		Key: "flagKey",
		On:  true,
		Rules: []Rule{
			Rule{
				ID:                 "rule-id",
				Clauses:            []Clause{makeClauseToMatchUser(evalTestUser)},
				VariationOrRollout: VariationOrRollout{Variation: intPtr(1)},
				TrackEvents:        true,
			},
		},
		Variations: []interface{}{"off", "on"},
		Version:    1,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	value, err := client.StringVariation("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(FeatureRequestEvent)
	assert.True(t, e.TrackEvents)
	assert.Equal(t, newEvalReasonRuleMatch(0, "rule-id"), e.Reason.Reason)
}

func TestEventTrackingAndReasonAreNotForcedIfFlagIsNotSetForMatchingRule(t *testing.T) {
	flag := FeatureFlag{
		Key: "flagKey",
		On:  true,
		Rules: []Rule{
			Rule{
				ID:                 "id0",
				Clauses:            []Clause{makeClauseToNotMatchUser(evalTestUser)},
				VariationOrRollout: VariationOrRollout{Variation: intPtr(0)},
				TrackEvents:        true,
			},
			Rule{
				ID:                 "id1",
				Clauses:            []Clause{makeClauseToMatchUser(evalTestUser)},
				VariationOrRollout: VariationOrRollout{Variation: intPtr(1)},
			},
		},
		Variations: []interface{}{"off", "on"},
		Version:    1,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	value, err := client.StringVariation("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Nil(t, e.Reason.Reason)
}

func TestEventTrackingAndReasonCanBeForcedForFallthrough(t *testing.T) {
	flag := FeatureFlag{
		Key:                    "flagKey",
		On:                     true,
		Fallthrough:            VariationOrRollout{Variation: intPtr(1)},
		Variations:             []interface{}{"off", "on"},
		TrackEventsFallthrough: true,
		Version:                1,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	value, err := client.StringVariation("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(FeatureRequestEvent)
	assert.True(t, e.TrackEvents)
	assert.Equal(t, evalReasonFallthroughInstance, e.Reason.Reason)
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfFlagIsNotSet(t *testing.T) {
	flag := FeatureFlag{
		Key:         "flagKey",
		On:          true,
		Fallthrough: VariationOrRollout{Variation: intPtr(1)},
		Variations:  []interface{}{"off", "on"},
		Version:     1,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	value, err := client.StringVariation("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Nil(t, e.Reason.Reason)
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfReasonIsNotFallthrough(t *testing.T) {
	flag := FeatureFlag{
		Key:                    "flagKey",
		On:                     false,
		OffVariation:           intPtr(0),
		Fallthrough:            VariationOrRollout{Variation: intPtr(1)},
		Variations:             []interface{}{"off", "on"},
		TrackEventsFallthrough: true,
		Version:                1,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	value, err := client.StringVariation("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "off", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Nil(t, e.Reason.Reason)
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
				"variation":0,"version":100
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

func TestAllFlagsStateGetsStateWithReasons(t *testing.T) {
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

	state := client.AllFlagsState(NewUser("userkey"), WithReasons)
	assert.True(t, state.IsValid())

	expectedString := `{
		"key1":"value1",
		"key2":"value2",
		"$flagsState":{
	  		"key1":{
				"variation":0,"version":100,"reason":{"kind":"OFF"}
			},
			"key2": {
				"variation":1,"version":200,"reason":{"kind":"OFF"},"trackEvents":true,"debugEventsUntilDate":1000
			}
		},
		"$valid":true
	}`
	actualBytes, err := json.Marshal(state)
	assert.NoError(t, err)
	assert.JSONEq(t, expectedString, string(actualBytes))
}

func TestAllFlagsStateCanOmitDetailForUntrackedFlags(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	futureTime := now() + 100000
	futureTimeStr := strconv.FormatInt(int64(futureTime), 10)
	flag1 := FeatureFlag{
		Key:          "key1",
		Version:      100,
		OffVariation: intPtr(0),
		Variations:   []interface{}{"value1"},
	}
	flag2 := FeatureFlag{
		Key:          "key2",
		Version:      200,
		OffVariation: intPtr(1),
		Variations:   []interface{}{"x", "value2"},
		TrackEvents:  true,
	}
	flag3 := FeatureFlag{
		Key:                  "key3",
		Version:              300,
		OffVariation:         intPtr(1),
		Variations:           []interface{}{"x", "value3"},
		TrackEvents:          false,
		DebugEventsUntilDate: &futureTime, // event tracking is turned on temporarily even though TrackEvents is false
	}
	client.store.Upsert(Features, &flag1)
	client.store.Upsert(Features, &flag2)
	client.store.Upsert(Features, &flag3)

	state := client.AllFlagsState(NewUser("userkey"), WithReasons, DetailsOnlyForTrackedFlags)
	assert.True(t, state.IsValid())

	expectedString := `{
		"key1":"value1",
		"key2":"value2",
		"key3":"value3",
		"$flagsState":{
	  		"key1":{
				"variation":0
			},
			"key2": {
				"variation":1,"version":200,"reason":{"kind":"OFF"},"trackEvents":true
			},
			"key3": {
				"variation":1,"version":300,"reason":{"kind":"OFF"},"debugEventsUntilDate":` + futureTimeStr + `
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

	flag1 := makeTestFlag("flag0", 0, "value1")
	flag2 := makeTestFlag("flag1", 0, "value2")
	client.store.Upsert(Features, flag1)
	client.store.Upsert(Features, flag2)

	state := client.AllFlagsState(evalTestUserWithNilKey)
	assert.False(t, state.IsValid())
	assert.Nil(t, state.ToValuesMap())
}

func TestUnknownFlagErrorLogging(t *testing.T) {
	testEvalErrorLogging(t, nil, "unknown-flag", evalTestUser,
		"WARN: unknown feature key: unknown-flag\\. Verify that this feature key exists\\. Returning default value")
}

func TestInvalidUserErrorLogging(t *testing.T) {
	testEvalErrorLogging(t, makeTestFlag("valid-flag", 1, false, true), "", evalTestUserWithNilKey,
		"WARN: user\\.Key cannot be nil when evaluating flag: valid-flag\\. Returning default value")
}

func TestMalformedFlagErrorLogging(t *testing.T) {
	testEvalErrorLogging(t, makeMalformedFlag("bad-flag"), "", evalTestUser,
		"WARN: flag evaluation for bad-flag failed with error MALFORMED_FLAG, default value was returned")
}

func testEvalErrorLogging(t *testing.T, flag *FeatureFlag, key string, user User, expectedMessageRegex string) {
	runTest := func(withLogging bool) {
		logger := newMockLogger("WARN:")
		client := makeTestClientWithConfig(func(c *Config) {
			c.Logger = logger
			c.LogEvaluationErrors = withLogging
		})
		defer client.Close()
		if flag != nil {
			client.store.Upsert(Features, flag)
			key = flag.Key
		}

		value, _ := client.StringVariation(key, user, "default")
		assert.Equal(t, "default", value)
		if withLogging {
			assert.Equal(t, 1, len(logger.output))
			assert.Regexp(t, expectedMessageRegex, logger.output[0])
		} else {
			assert.Equal(t, 0, len(logger.output))
		}
	}
	runTest(false)
	runTest(true)
}

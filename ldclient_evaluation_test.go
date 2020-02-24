package ldclient

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldevents"
)

var evalTestUser = lduser.NewUser("userkey")

var fallthroughValue = ldvalue.String("fall")
var offValue = ldvalue.String("off")
var onValue = ldvalue.String("on")

func makeTestFlag(key string, fallThroughVariation int, variations ...ldvalue.Value) *ldeval.FeatureFlag {
	return &ldeval.FeatureFlag{
		Key:         key,
		Version:     1,
		On:          true,
		Fallthrough: ldeval.VariationOrRollout{Variation: &fallThroughVariation},
		Variations:  variations,
	}
}

func makeMalformedFlag(key string) *ldeval.FeatureFlag {
	return &ldeval.FeatureFlag{Key: key, On: false, OffVariation: intPtr(-1)}
}

func makeClauseToMatchUser(user lduser.User) ldeval.Clause {
	return ldeval.Clause{
		Attribute: "key",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.String(user.GetKey())},
	}
}

func makeClauseToNotMatchUser(user lduser.User) ldeval.Clause {
	return ldeval.Clause{
		Attribute: "key",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.String("not-" + user.GetKey())},
	}
}

func intPtr(n int) *int {
	return &n
}

func assertEvalEvent(t *testing.T, client *LDClient, flag *ldeval.FeatureFlag, user lduser.User, value ldvalue.Value,
	variation int, defaultVal ldvalue.Value, reason ldreason.EvaluationReason) {
	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.FeatureRequestEvent)
	expectedEvent := ldevents.FeatureRequestEvent{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: e.CreationDate,
			User:         user,
		},
		Key:       flag.Key,
		Version:   flag.Version,
		Value:     value,
		Variation: variation,
		Default:   defaultVal,
		Reason:    reason,
	}
	assert.Equal(t, expectedEvent, e)
}

func TestBoolVariation(t *testing.T) {
	expected := true
	defaultVal := false
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.Bool(false), ldvalue.Bool(true))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.BoolVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Bool(expected), 1, ldvalue.Bool(defaultVal), ldreason.EvaluationReason{})
}

func TestBoolVariationDetail(t *testing.T) {
	expected := true
	defaultVal := false
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.Bool(false), ldvalue.Bool(true))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.BoolVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.BoolValue())
	assert.Equal(t, 1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Bool(expected), 1, ldvalue.Bool(defaultVal), detail.Reason)
}

func TestIntVariation(t *testing.T) {
	expected := 100
	defaultVal := 10000
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.Int(-1), ldvalue.Int(expected))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.IntVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, int(expected), actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Int(expected), 1, ldvalue.Int(defaultVal), ldreason.EvaluationReason{})
}

func TestIntVariationRoundsFloatTowardZero(t *testing.T) {
	flag1 := makeTestFlag("flag1", 1, ldvalue.Float64(-1), ldvalue.Float64(2.25))
	flag2 := makeTestFlag("flag2", 1, ldvalue.Float64(-1), ldvalue.Float64(2.75))
	flag3 := makeTestFlag("flag3", 1, ldvalue.Float64(-1), ldvalue.Float64(-2.25))
	flag4 := makeTestFlag("flag4", 1, ldvalue.Float64(-1), ldvalue.Float64(-2.75))

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
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.Int(-1), ldvalue.Int(expected))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.IntVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.IntValue())
	assert.Equal(t, 1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Int(expected), 1, ldvalue.Int(defaultVal), detail.Reason)
}

func TestFloat64Variation(t *testing.T) {
	expected := 100.01
	defaultVal := 0.0
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.Float64(-1.0), ldvalue.Float64(expected))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.Float64Variation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Float64(expected), 1, ldvalue.Float64(defaultVal), ldreason.EvaluationReason{})
}

func TestFloat64VariationDetail(t *testing.T) {
	expected := 100.01
	defaultVal := 0.0
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.Float64(-1.0), ldvalue.Float64(expected))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.Float64VariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.Float64Value())
	assert.Equal(t, 1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Float64(expected), 1, ldvalue.Float64(defaultVal), detail.Reason)
}

func TestStringVariation(t *testing.T) {
	expected := "b"
	defaultVal := "a"
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.String("a"), ldvalue.String("b"))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, err := client.StringVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.String(expected), 1, ldvalue.String(defaultVal), ldreason.EvaluationReason{})
}

func TestStringVariationDetail(t *testing.T) {
	expected := "b"
	defaultVal := "a"
	flag := makeTestFlag("validFeatureKey", 1, ldvalue.String("a"), ldvalue.String("b"))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	actual, detail, err := client.StringVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.StringValue())
	assert.Equal(t, 1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.String(expected), 1, ldvalue.String(defaultVal), detail.Reason)
}

func TestJSONRawVariation(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	otherValue := map[string]interface{}{"field1": "value1"}
	expectedJSON, _ := json.Marshal(expectedValue)

	flag := makeTestFlag("validFeatureKey", 1,
		ldvalue.CopyArbitraryValue(otherValue), ldvalue.CopyArbitraryValue(expectedValue))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, err := client.JSONVariation("validFeatureKey", evalTestUser, ldvalue.Raw(defaultVal))

	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(expectedJSON), actual.AsRaw())

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), 1,
		ldvalue.CopyArbitraryValue(defaultVal), ldreason.EvaluationReason{})
}

func TestJSONRawVariationDetail(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	otherValue := map[string]interface{}{"field1": "value1"}
	expectedJSON, _ := json.Marshal(expectedValue)
	expectedRaw := json.RawMessage(expectedJSON)

	flag := makeTestFlag("validFeatureKey", 1,
		ldvalue.CopyArbitraryValue(otherValue), ldvalue.CopyArbitraryValue(expectedValue))

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, detail, err := client.JSONVariationDetail("validFeatureKey", evalTestUser, ldvalue.Raw(defaultVal))

	assert.NoError(t, err)
	assert.Equal(t, expectedRaw, actual.AsRaw())
	assert.Equal(t, expectedRaw, detail.Value.AsRaw())
	assert.Equal(t, 1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), 1, ldvalue.CopyArbitraryValue(defaultVal), detail.Reason)
}

func TestJSONVariation(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})
	otherValue := ldvalue.CopyArbitraryValue(map[string]interface{}{"field1": "value1"})

	flag := makeTestFlag("validFeatureKey", 1, otherValue, expected)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := ldvalue.String("no")
	actual, err := client.JSONVariation("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected.AsArbitraryValue(), actual.AsArbitraryValue()) // assert.Equal isn't currently reliable for complex Value types

	assertEvalEvent(t, client, flag, evalTestUser, expected, 1, defaultVal, ldreason.EvaluationReason{})
}

func TestJSONVariationDetail(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})
	otherValue := ldvalue.CopyArbitraryValue(map[string]interface{}{"field1": "value1"})

	flag := makeTestFlag("validFeatureKey", 1, otherValue, expected)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	defaultVal := ldvalue.String("no")
	actual, detail, err := client.JSONVariationDetail("validFeatureKey", evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value)
	assert.Equal(t, 1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), detail.Reason)

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
	assert.Equal(t, ldvalue.String("default"), detail.Value)
	assert.Equal(t, -1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonError(ldreason.EvalErrorFlagNotFound), detail.Reason)
	assert.True(t, detail.IsDefaultValue())
}

func TestDefaultIsReturnedIfFlagEvaluatesToNil(t *testing.T) {
	flag := ldeval.FeatureFlag{
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
	flag := ldeval.FeatureFlag{
		Key:          "flagKey",
		On:           false,
		OffVariation: nil,
	}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, &flag)

	_, detail, err := client.StringVariationDetail("flagKey", evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, ldvalue.String("default"), detail.Value)
	assert.Equal(t, -1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonOff(), detail.Reason)
}

func TestEventTrackingAndReasonCanBeForcedForRule(t *testing.T) {
	flag := ldeval.FeatureFlag{
		Key: "flagKey",
		On:  true,
		Rules: []ldeval.FlagRule{
			ldeval.FlagRule{
				ID:                 "rule-id",
				Clauses:            []ldeval.Clause{makeClauseToMatchUser(evalTestUser)},
				VariationOrRollout: ldeval.VariationOrRollout{Variation: intPtr(1)},
				TrackEvents:        true,
			},
		},
		Variations: []ldvalue.Value{offValue, onValue},
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

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.True(t, e.TrackEvents)
	assert.Equal(t, ldreason.NewEvalReasonRuleMatch(0, "rule-id"), e.Reason)
}

func TestEventTrackingAndReasonAreNotForcedIfFlagIsNotSetForMatchingRule(t *testing.T) {
	flag := ldeval.FeatureFlag{
		Key: "flagKey",
		On:  true,
		Rules: []ldeval.FlagRule{
			ldeval.FlagRule{
				ID:                 "id0",
				Clauses:            []ldeval.Clause{makeClauseToNotMatchUser(evalTestUser)},
				VariationOrRollout: ldeval.VariationOrRollout{Variation: intPtr(0)},
				TrackEvents:        true,
			},
			ldeval.FlagRule{
				ID:                 "id1",
				Clauses:            []ldeval.Clause{makeClauseToMatchUser(evalTestUser)},
				VariationOrRollout: ldeval.VariationOrRollout{Variation: intPtr(1)},
			},
		},
		Variations: []ldvalue.Value{offValue, onValue},
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

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
}

func TestEventTrackingAndReasonCanBeForcedForFallthrough(t *testing.T) {
	flag := ldeval.FeatureFlag{
		Key:                    "flagKey",
		On:                     true,
		Fallthrough:            ldeval.VariationOrRollout{Variation: intPtr(1)},
		Variations:             []ldvalue.Value{offValue, onValue},
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

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.True(t, e.TrackEvents)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), e.Reason)
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfFlagIsNotSet(t *testing.T) {
	flag := ldeval.FeatureFlag{
		Key:         "flagKey",
		On:          true,
		Fallthrough: ldeval.VariationOrRollout{Variation: intPtr(1)},
		Variations:  []ldvalue.Value{offValue, onValue},
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

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfReasonIsNotFallthrough(t *testing.T) {
	flag := ldeval.FeatureFlag{
		Key:                    "flagKey",
		On:                     false,
		OffVariation:           intPtr(0),
		Fallthrough:            ldeval.VariationOrRollout{Variation: intPtr(1)},
		Variations:             []ldvalue.Value{offValue, onValue},
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

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
}

func TestEvaluatingUnknownFlagSendsEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	_, err := client.StringVariation("flagKey", evalTestUser, "x")
	assert.Error(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(ldevents.FeatureRequestEvent)
	expectedEvent := ldevents.FeatureRequestEvent{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: e.CreationDate,
			User:         evalTestUser,
		},
		Key:       "flagKey",
		Version:   ldevents.NoVersion,
		Value:     ldvalue.String("x"),
		Variation: ldevents.NoVariation,
		Default:   ldvalue.String("x"),
	}
	assert.Equal(t, expectedEvent, e)
}

func TestEvaluatingFlagWithPrerequisiteSendsPrerequisiteEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag0 := makeTestFlag("flag0", 1, ldvalue.String("a"), ldvalue.String("b"))
	flag0.Prerequisites = []ldeval.Prerequisite{
		ldeval.Prerequisite{Key: "flag1", Variation: 1},
	}
	flag1 := makeTestFlag("flag1", 1, ldvalue.String("c"), ldvalue.String("d"))
	client.store.Upsert(Features, flag0)
	client.store.Upsert(Features, flag1)

	user := lduser.NewUser("userKey")
	_, err := client.StringVariation(flag0.Key, user, "x")
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 2, len(events))

	e0 := events[0].(ldevents.FeatureRequestEvent)
	expected0 := ldevents.FeatureRequestEvent{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: e0.CreationDate,
			User:         user,
		},
		Key:       flag1.Key,
		Version:   flag1.Version,
		Value:     ldvalue.String("d"),
		Variation: 1,
		Default:   ldvalue.Null(),
		PrereqOf:  ldvalue.NewOptionalString(flag0.Key),
	}
	assert.Equal(t, expected0, e0)

	e1 := events[1].(ldevents.FeatureRequestEvent)
	expected1 := ldevents.FeatureRequestEvent{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: e1.CreationDate,
			User:         user,
		},
		Key:       flag0.Key,
		Version:   flag0.Version,
		Value:     ldvalue.String("b"),
		Variation: 1,
		Default:   ldvalue.String("x"),
	}
	assert.Equal(t, expected1, e1)
}

func TestAllFlagsStateGetsState(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag1 := ldeval.FeatureFlag{
		Key:          "key1",
		Version:      100,
		OffVariation: intPtr(0),
		Variations:   []ldvalue.Value{ldvalue.String("value1")},
	}
	date := uint64(1000)
	flag2 := ldeval.FeatureFlag{
		Key:                  "key2",
		Version:              200,
		OffVariation:         intPtr(1),
		Variations:           []ldvalue.Value{ldvalue.String("x"), ldvalue.String("value2")},
		TrackEvents:          true,
		DebugEventsUntilDate: &date,
	}
	client.store.Upsert(Features, &flag1)
	client.store.Upsert(Features, &flag2)

	state := client.AllFlagsState(lduser.NewUser("userkey"))
	assert.True(t, state.IsValid())

	expectedString := `{
		"key1":"value1",
		"key2":"value2",
		"$flagsState":{
	  		"key1":{
				"variation":0,"version":100,"reason":null
			},
			"key2": {
				"variation":1,"version":200,"trackEvents":true,"debugEventsUntilDate":1000,"reason":null
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

	flag1 := ldeval.FeatureFlag{Key: "server-side-1"}
	flag2 := ldeval.FeatureFlag{Key: "server-side-2"}
	flag3 := ldeval.FeatureFlag{
		Key:          "client-side-1",
		OffVariation: intPtr(0),
		Variations:   []ldvalue.Value{ldvalue.String("value1")},
		ClientSide:   true,
	}
	flag4 := ldeval.FeatureFlag{
		Key:          "client-side-2",
		OffVariation: intPtr(0),
		Variations:   []ldvalue.Value{ldvalue.String("value2")},
		ClientSide:   true,
	}
	client.store.Upsert(Features, &flag1)
	client.store.Upsert(Features, &flag2)
	client.store.Upsert(Features, &flag3)
	client.store.Upsert(Features, &flag4)

	state := client.AllFlagsState(lduser.NewUser("userkey"), ClientSideOnly)
	assert.True(t, state.IsValid())

	expectedValues := map[string]ldvalue.Value{"client-side-1": ldvalue.String("value1"), "client-side-2": ldvalue.String("value2")}
	assert.Equal(t, expectedValues, state.ToValuesMap())
}

func TestAllFlagsStateGetsStateWithReasons(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag1 := ldeval.FeatureFlag{
		Key:          "key1",
		Version:      100,
		OffVariation: intPtr(0),
		Variations:   []ldvalue.Value{ldvalue.String("value1")},
	}
	date := uint64(1000)
	flag2 := ldeval.FeatureFlag{
		Key:                  "key2",
		Version:              200,
		OffVariation:         intPtr(1),
		Variations:           []ldvalue.Value{ldvalue.String("x"), ldvalue.String("value2")},
		TrackEvents:          true,
		DebugEventsUntilDate: &date,
	}
	client.store.Upsert(Features, &flag1)
	client.store.Upsert(Features, &flag2)

	state := client.AllFlagsState(lduser.NewUser("userkey"), WithReasons)
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
	flag1 := ldeval.FeatureFlag{
		Key:          "key1",
		Version:      100,
		OffVariation: intPtr(0),
		Variations:   []ldvalue.Value{ldvalue.String("value1")},
	}
	flag2 := ldeval.FeatureFlag{
		Key:          "key2",
		Version:      200,
		OffVariation: intPtr(1),
		Variations:   []ldvalue.Value{ldvalue.String("x"), ldvalue.String("value2")},
		TrackEvents:  true,
	}
	flag3 := ldeval.FeatureFlag{
		Key:                  "key3",
		Version:              300,
		OffVariation:         intPtr(1),
		Variations:           []ldvalue.Value{ldvalue.String("x"), ldvalue.String("value3")},
		TrackEvents:          false,
		DebugEventsUntilDate: &futureTime, // event tracking is turned on temporarily even though TrackEvents is false
	}
	client.store.Upsert(Features, &flag1)
	client.store.Upsert(Features, &flag2)
	client.store.Upsert(Features, &flag3)

	state := client.AllFlagsState(lduser.NewUser("userkey"), WithReasons, DetailsOnlyForTrackedFlags)
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

func TestUnknownFlagErrorLogging(t *testing.T) {
	testEvalErrorLogging(t, nil, "unknown-flag", evalTestUser,
		"WARN: unknown feature key: unknown-flag\\. Verify that this feature key exists\\. Returning default value")
}

func TestMalformedFlagErrorLogging(t *testing.T) {
	testEvalErrorLogging(t, makeMalformedFlag("bad-flag"), "", evalTestUser,
		"WARN: flag evaluation for bad-flag failed with error MALFORMED_FLAG, default value was returned")
}

func testEvalErrorLogging(t *testing.T, flag *ldeval.FeatureFlag, key string, user lduser.User, expectedMessageRegex string) {
	runTest := func(withLogging bool) {
		logger := newMockLogger("WARN:")
		client := makeTestClientWithConfig(func(c *Config) {
			c.Loggers.SetBaseLogger(logger)
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

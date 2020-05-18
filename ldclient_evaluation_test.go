package ldclient

import (
	"encoding/json"
	"strconv"
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"

	helpers "github.com/launchdarkly/go-test-helpers"
	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
)

var evalTestUser = lduser.NewUser("userkey")

var fallthroughValue = ldvalue.String("fall")
var offValue = ldvalue.String("off")
var onValue = ldvalue.String("on")

const expectedVariationForSingleValueFlag = 1

var expectedReasonForSingleValueFlag = ldreason.NewEvalReasonOff()
var noReason = ldreason.EvaluationReason{}

// Note that we use this function instead of ldbuilders.FlagBuilder.SingleVariation() because we want to make sure that
// our test flags return a variation index of 1 rather than 0, so we can be sure that the variation index is actually
// being copied into the detail object and the event.
func singleValueFlag(key string, value ldvalue.Value) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).OffVariation(1).Variations(ldvalue.String("ignore-me"), value).Build()
}

func makeMalformedFlag(key string) *ldmodel.FeatureFlag {
	return &ldmodel.FeatureFlag{Key: key, On: false, OffVariation: helpers.IntPtr(-1)}
}

func makeClauseToMatchUser(user lduser.User) ldmodel.Clause {
	return ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String(user.GetKey()))
}

func makeClauseToNotMatchUser(user lduser.User) ldmodel.Clause {
	return ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String("not-"+user.GetKey()))
}

func assertEvalEvent(t *testing.T, client *LDClient, flag ldmodel.FeatureFlag, user lduser.User, value ldvalue.Value,
	variation int, defaultVal ldvalue.Value, reason ldreason.EvaluationReason) {
	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.FeatureRequestEvent)
	expectedEvent := ldevents.FeatureRequestEvent{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: e.CreationDate,
			User:         ldevents.User(user),
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
	flag := singleValueFlag("flagKey", ldvalue.Bool(true))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, err := client.BoolVariation(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Bool(expected), expectedVariationForSingleValueFlag, ldvalue.Bool(defaultVal), noReason)
}

func TestBoolVariationDetail(t *testing.T) {
	expected := true
	defaultVal := false
	flag := singleValueFlag("flagKey", ldvalue.Bool(true))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, detail, err := client.BoolVariationDetail(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.BoolValue())
	assert.Equal(t, expectedVariationForSingleValueFlag, detail.VariationIndex)
	assert.Equal(t, expectedReasonForSingleValueFlag, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Bool(expected), expectedVariationForSingleValueFlag, ldvalue.Bool(defaultVal), detail.Reason)
}

func TestIntVariation(t *testing.T) {
	expected := 100
	defaultVal := 10000
	flag := singleValueFlag("flagKey", ldvalue.Int(expected))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, err := client.IntVariation(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, int(expected), actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Int(expected), expectedVariationForSingleValueFlag, ldvalue.Int(defaultVal), noReason)
}

func TestIntVariationRoundsFloatTowardZero(t *testing.T) {
	flag1 := singleValueFlag("flag1", ldvalue.Float64(2.25))
	flag2 := singleValueFlag("flag2", ldvalue.Float64(2.75))
	flag3 := singleValueFlag("flag3", ldvalue.Float64(-2.25))
	flag4 := singleValueFlag("flag4", ldvalue.Float64(-2.75))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag1)
	upsertFlag(client.store, &flag2)
	upsertFlag(client.store, &flag3)
	upsertFlag(client.store, &flag4)

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
	flag := singleValueFlag("flagKey", ldvalue.Int(expected))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, detail, err := client.IntVariationDetail(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.IntValue())
	assert.Equal(t, expectedVariationForSingleValueFlag, detail.VariationIndex)
	assert.Equal(t, expectedReasonForSingleValueFlag, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Int(expected), expectedVariationForSingleValueFlag, ldvalue.Int(defaultVal), detail.Reason)
}

func TestFloat64Variation(t *testing.T) {
	expected := 100.01
	defaultVal := 0.0
	flag := singleValueFlag("flagKey", ldvalue.Float64(expected))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, err := client.Float64Variation(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Float64(expected), expectedVariationForSingleValueFlag, ldvalue.Float64(defaultVal), noReason)
}

func TestFloat64VariationDetail(t *testing.T) {
	expected := 100.01
	defaultVal := 0.0
	flag := singleValueFlag("flagKey", ldvalue.Float64(expected))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, detail, err := client.Float64VariationDetail(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.Float64Value())
	assert.Equal(t, expectedVariationForSingleValueFlag, detail.VariationIndex)
	assert.Equal(t, expectedReasonForSingleValueFlag, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.Float64(expected), expectedVariationForSingleValueFlag, ldvalue.Float64(defaultVal), detail.Reason)
}

func TestStringVariation(t *testing.T) {
	expected := "b"
	defaultVal := "a"
	flag := singleValueFlag("flagKey", ldvalue.String(expected))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, err := client.StringVariation(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.String(expected), expectedVariationForSingleValueFlag, ldvalue.String(defaultVal), noReason)
}

func TestStringVariationDetail(t *testing.T) {
	expected := "b"
	defaultVal := "a"
	flag := singleValueFlag("flagKey", ldvalue.String(expected))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	actual, detail, err := client.StringVariationDetail(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value.StringValue())
	assert.Equal(t, expectedVariationForSingleValueFlag, detail.VariationIndex)
	assert.Equal(t, expectedReasonForSingleValueFlag, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.String(expected), expectedVariationForSingleValueFlag, ldvalue.String(defaultVal), detail.Reason)
}

func TestJSONRawVariation(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	expectedJSON, _ := json.Marshal(expectedValue)

	flag := singleValueFlag("flagKey", ldvalue.CopyArbitraryValue(expectedValue))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, err := client.JSONVariation(flag.Key, evalTestUser, ldvalue.Raw(defaultVal))

	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(expectedJSON), actual.AsRaw())

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), expectedVariationForSingleValueFlag,
		ldvalue.CopyArbitraryValue(defaultVal), noReason)
}

func TestJSONRawVariationDetail(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	expectedJSON, _ := json.Marshal(expectedValue)
	expectedRaw := json.RawMessage(expectedJSON)

	flag := singleValueFlag("flagKey", ldvalue.CopyArbitraryValue(expectedValue))

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	actual, detail, err := client.JSONVariationDetail(flag.Key, evalTestUser, ldvalue.Raw(defaultVal))

	assert.NoError(t, err)
	assert.Equal(t, expectedRaw, actual.AsRaw())
	assert.Equal(t, expectedRaw, detail.Value.AsRaw())
	assert.Equal(t, expectedVariationForSingleValueFlag, detail.VariationIndex)
	assert.Equal(t, expectedReasonForSingleValueFlag, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, ldvalue.CopyArbitraryValue(expectedValue), expectedVariationForSingleValueFlag,
		ldvalue.CopyArbitraryValue(defaultVal), detail.Reason)
}

func TestJSONVariation(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})

	flag := singleValueFlag("flagKey", expected)

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	defaultVal := ldvalue.String("no")
	actual, err := client.JSONVariation(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	assertEvalEvent(t, client, flag, evalTestUser, expected, expectedVariationForSingleValueFlag, defaultVal, noReason)
}

func TestJSONVariationDetail(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})

	flag := singleValueFlag("flagKey", expected)

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	defaultVal := ldvalue.String("no")
	actual, detail, err := client.JSONVariationDetail(flag.Key, evalTestUser, defaultVal)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expected, detail.Value)
	assert.Equal(t, expectedVariationForSingleValueFlag, detail.VariationIndex)
	assert.Equal(t, expectedReasonForSingleValueFlag, detail.Reason)

	assertEvalEvent(t, client, flag, evalTestUser, expected, expectedVariationForSingleValueFlag, defaultVal, detail.Reason)
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
	flag := ldbuilders.NewFlagBuilder("flagKey").Build() // flag is off and we haven't defined an off variation

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	value, err := client.StringVariation(flag.Key, evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "default", value)
}

func TestDefaultIsReturnedIfFlagEvaluatesToNilWithDetail(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").Build() // flag is off and we haven't defined an off variation

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	_, detail, err := client.StringVariationDetail(flag.Key, evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, ldvalue.String("default"), detail.Value)
	assert.Equal(t, -1, detail.VariationIndex)
	assert.Equal(t, ldreason.NewEvalReasonOff(), detail.Reason)
}

func TestEventTrackingAndReasonCanBeForcedForRule(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").
		On(true).
		AddRule(ldbuilders.NewRuleBuilder().
			ID("rule-id").
			Clauses(makeClauseToMatchUser(evalTestUser)).
			Variation(1).
			TrackEvents(true)).
		Variations(offValue, onValue).
		Version(1).
		Build()

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	value, err := client.StringVariation(flag.Key, evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.True(t, e.TrackEvents)
	assert.Equal(t, ldreason.NewEvalReasonRuleMatch(0, "rule-id"), e.Reason)
}

func TestEventTrackingAndReasonAreNotForcedIfFlagIsNotSetForMatchingRule(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").
		On(true).
		AddRule(ldbuilders.NewRuleBuilder().
			ID("id0").
			Clauses(makeClauseToNotMatchUser(evalTestUser)).
			Variation(0).
			TrackEvents(true)).
		AddRule(ldbuilders.NewRuleBuilder().
			ID("id1").
			Clauses(makeClauseToMatchUser(evalTestUser)).
			Variation(1)).
		Variations(offValue, onValue).
		Version(1).
		Build()

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	value, err := client.StringVariation(flag.Key, evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
}

func TestEventTrackingAndReasonCanBeForcedForFallthrough(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").
		On(true).
		FallthroughVariation(1).
		Variations(offValue, onValue).
		TrackEventsFallthrough(true).
		Version(1).
		Build()

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	value, err := client.StringVariation(flag.Key, evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.True(t, e.TrackEvents)
	assert.Equal(t, ldreason.NewEvalReasonFallthrough(), e.Reason)
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfFlagIsNotSet(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").
		On(true).
		FallthroughVariation(1).
		Variations(offValue, onValue).
		Version(1).
		Build()

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	value, err := client.StringVariation(flag.Key, evalTestUser, "default")
	assert.NoError(t, err)
	assert.Equal(t, "on", value)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(ldevents.FeatureRequestEvent)
	assert.False(t, e.TrackEvents)
	assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfReasonIsNotFallthrough(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").
		On(false).
		OffVariation(0).
		FallthroughVariation(1).
		Variations(offValue, onValue).
		TrackEventsFallthrough(true).
		Version(1).
		Build()

	client := makeTestClient()
	defer client.Close()
	upsertFlag(client.store, &flag)

	value, err := client.StringVariation(flag.Key, evalTestUser, "default")
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
			User:         ldevents.User(evalTestUser),
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

	flag0 := ldbuilders.NewFlagBuilder("flag0").
		On(true).
		FallthroughVariation(1).
		Variations(ldvalue.String("a"), ldvalue.String("b")).
		AddPrerequisite("flag1", 1).
		Build()
	flag1 := ldbuilders.NewFlagBuilder("flag1").
		On(true).
		FallthroughVariation(1).
		Variations(ldvalue.String("c"), ldvalue.String("d")).
		Build()
	upsertFlag(client.store, &flag0)
	upsertFlag(client.store, &flag1)

	user := lduser.NewUser("userKey")
	_, err := client.StringVariation(flag0.Key, user, "x")
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 2, len(events))

	e0 := events[0].(ldevents.FeatureRequestEvent)
	expected0 := ldevents.FeatureRequestEvent{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: e0.CreationDate,
			User:         ldevents.User(user),
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
			User:         ldevents.User(user),
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

	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).DebugEventsUntilDate(1000).Build()
	upsertFlag(client.store, &flag1)
	upsertFlag(client.store, &flag2)

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

	flag1 := ldbuilders.NewFlagBuilder("server-side-1").Build()
	flag2 := ldbuilders.NewFlagBuilder("server-side-2").Build()
	flag3 := ldbuilders.NewFlagBuilder("client-side-1").SingleVariation(ldvalue.String("value1")).ClientSide(true).Build()
	flag4 := ldbuilders.NewFlagBuilder("client-side-2").SingleVariation(ldvalue.String("value2")).ClientSide(true).Build()
	upsertFlag(client.store, &flag1)
	upsertFlag(client.store, &flag2)
	upsertFlag(client.store, &flag3)
	upsertFlag(client.store, &flag4)

	state := client.AllFlagsState(lduser.NewUser("userkey"), ClientSideOnly)
	assert.True(t, state.IsValid())

	expectedValues := map[string]ldvalue.Value{"client-side-1": ldvalue.String("value1"), "client-side-2": ldvalue.String("value2")}
	assert.Equal(t, expectedValues, state.ToValuesMap())
}

func TestAllFlagsStateGetsStateWithReasons(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).DebugEventsUntilDate(1000).Build()
	upsertFlag(client.store, &flag1)
	upsertFlag(client.store, &flag2)

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

	futureTime := ldtime.UnixMillisNow() + 100000
	futureTimeStr := strconv.FormatInt(int64(futureTime), 10)
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).Build()
	flag3 := ldbuilders.NewFlagBuilder("key3").Version(300).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value3")).
		TrackEvents(false).DebugEventsUntilDate(futureTime).Build()
	upsertFlag(client.store, &flag1)
	upsertFlag(client.store, &flag2)
	upsertFlag(client.store, &flag3)

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
		"unknown feature key: unknown-flag\\. Verify that this feature key exists\\. Returning default value")
}

func TestMalformedFlagErrorLogging(t *testing.T) {
	testEvalErrorLogging(t, makeMalformedFlag("bad-flag"), "", evalTestUser,
		"flag evaluation for bad-flag failed with error MALFORMED_FLAG, default value was returned")
}

func testEvalErrorLogging(t *testing.T, flag *ldmodel.FeatureFlag, key string, user lduser.User, expectedMessageRegex string) {
	runTest := func(withLogging bool) {
		mockLoggers := sharedtest.NewMockLoggers()
		client := makeTestClientWithConfig(func(c *Config) {
			c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Warn).LogEvaluationErrors(withLogging)
		})
		defer client.Close()
		if flag != nil {
			upsertFlag(client.store, flag)
			key = flag.Key
		}

		value, _ := client.StringVariation(key, user, "default")
		assert.Equal(t, "default", value)
		if withLogging {
			assert.Equal(t, 1, len(mockLoggers.AllOutput))
			assert.Regexp(t, expectedMessageRegex, mockLoggers.Output[ldlog.Warn][0])
		} else {
			assert.Equal(t, 0, len(mockLoggers.AllOutput))
		}
	}
	runTest(false)
	runTest(true)
}

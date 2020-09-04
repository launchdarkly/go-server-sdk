package ldclient

import (
	"encoding/json"
	"errors"
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func makeClauseToMatchUser(user lduser.User) ldmodel.Clause {
	return ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String(user.GetKey()))
}

func makeClauseToNotMatchUser(user lduser.User) ldmodel.Clause {
	return ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String("not-"+user.GetKey()))
}

type clientEvalTestParams struct {
	client  *LDClient
	store   interfaces.DataStore
	events  *sharedtest.CapturingEventProcessor
	mockLog *ldlogtest.MockLog
}

func withClientEvalTestParams(callback func(clientEvalTestParams)) {
	p := clientEvalTestParams{}
	p.store = datastore.NewInMemoryDataStore(ldlog.NewDisabledLoggers())
	p.events = &sharedtest.CapturingEventProcessor{}
	p.mockLog = ldlogtest.NewMockLog()
	config := Config{
		Offline:   false,
		DataStore: sharedtest.SingleDataStoreFactory{Instance: p.store},
		DataSource: sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: true},
		},
		Events:  sharedtest.SingleEventProcessorFactory{Instance: p.events},
		Logging: ldcomponents.Logging().Loggers(p.mockLog.Loggers),
	}
	p.client, _ = MakeCustomClient("sdk_key", config, 0)
	defer p.client.Close()
	callback(p)
}

func (p clientEvalTestParams) requireSingleEvent(t *testing.T) ldevents.FeatureRequestEvent {
	events := p.events.Events
	require.Equal(t, 1, len(events))
	return events[0].(ldevents.FeatureRequestEvent)
}

func assertEvalEvent(
	t *testing.T,
	actualEvent ldevents.FeatureRequestEvent,
	flag ldmodel.FeatureFlag,
	user lduser.User,
	value ldvalue.Value,
	variation int,
	defaultVal ldvalue.Value,
	reason ldreason.EvaluationReason,
) {
	expectedEvent := ldevents.FeatureRequestEvent{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: actualEvent.CreationDate,
			User:         ldevents.User(user),
		},
		Key:       flag.Key,
		Version:   ldvalue.NewOptionalInt(flag.Version),
		Value:     value,
		Variation: ldvalue.NewOptionalInt(variation),
		Default:   defaultVal,
		Reason:    reason,
	}
	assert.Equal(t, expectedEvent, actualEvent)
}

func TestBoolVariation(t *testing.T) {
	expected := true
	defaultVal := false
	flag := singleValueFlag("flagKey", ldvalue.Bool(true))

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, err := p.client.BoolVariation(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.Bool(expected),
				expectedVariationForSingleValueFlag, ldvalue.Bool(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, detail, err := p.client.BoolVariationDetail(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Bool(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.Bool(expected),
				expectedVariationForSingleValueFlag, ldvalue.Bool(defaultVal), detail.Reason)
		})
	})
}

func TestIntVariation(t *testing.T) {
	expected := 100
	defaultVal := 10000
	flag := singleValueFlag("flagKey", ldvalue.Int(expected))

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, err := p.client.IntVariation(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.Int(expected),
				expectedVariationForSingleValueFlag, ldvalue.Int(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, detail, err := p.client.IntVariationDetail(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Int(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.Int(expected),
				expectedVariationForSingleValueFlag, ldvalue.Int(defaultVal), detail.Reason)
		})
	})

	t.Run("rounds float toward zero", func(t *testing.T) {
		flag1 := singleValueFlag("flag1", ldvalue.Float64(2.25))
		flag2 := singleValueFlag("flag2", ldvalue.Float64(2.75))
		flag3 := singleValueFlag("flag3", ldvalue.Float64(-2.25))
		flag4 := singleValueFlag("flag4", ldvalue.Float64(-2.75))

		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag1)
			sharedtest.UpsertFlag(p.store, &flag2)
			sharedtest.UpsertFlag(p.store, &flag3)
			sharedtest.UpsertFlag(p.store, &flag4)

			actual, err := p.client.IntVariation(flag1.Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, 2, actual)

			actual, err = p.client.IntVariation(flag2.Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, 2, actual)

			actual, err = p.client.IntVariation(flag3.Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, -2, actual)

			actual, err = p.client.IntVariation(flag4.Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, -2, actual)
		})
	})
}

func TestFloat64Variation(t *testing.T) {
	expected := 100.01
	defaultVal := 0.0
	flag := singleValueFlag("flagKey", ldvalue.Float64(expected))

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, err := p.client.Float64Variation(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.Float64(expected),
				expectedVariationForSingleValueFlag, ldvalue.Float64(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, detail, err := p.client.Float64VariationDetail(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Float64(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.Float64(expected),
				expectedVariationForSingleValueFlag, ldvalue.Float64(defaultVal), detail.Reason)
		})
	})
}

func TestStringVariation(t *testing.T) {
	expected := "b"
	defaultVal := "a"
	flag := singleValueFlag("flagKey", ldvalue.String(expected))

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, err := p.client.StringVariation(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.String(expected),
				expectedVariationForSingleValueFlag, ldvalue.String(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, detail, err := p.client.StringVariationDetail(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.String(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, ldvalue.String(expected),
				expectedVariationForSingleValueFlag, ldvalue.String(defaultVal), detail.Reason)
		})
	})
}

func TestJSONRawVariation(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	expectedJSON, _ := json.Marshal(expectedValue)
	expectedRaw := json.RawMessage(expectedJSON)
	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))
	flag := singleValueFlag("flagKey", ldvalue.CopyArbitraryValue(expectedValue))

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, err := p.client.JSONVariation(flag.Key, evalTestUser, ldvalue.Raw(defaultVal))

			assert.NoError(t, err)
			assert.Equal(t, expectedRaw, actual.AsRaw())

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser,
				ldvalue.CopyArbitraryValue(expectedValue), expectedVariationForSingleValueFlag,
				ldvalue.CopyArbitraryValue(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, detail, err := p.client.JSONVariationDetail(flag.Key, evalTestUser, ldvalue.Raw(defaultVal))

			assert.NoError(t, err)
			assert.Equal(t, expectedRaw, actual.AsRaw())
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Parse(expectedRaw), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser,
				ldvalue.CopyArbitraryValue(expectedValue), expectedVariationForSingleValueFlag,
				ldvalue.CopyArbitraryValue(defaultVal), detail.Reason)
		})
	})
}

func TestJSONVariation(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})
	defaultVal := ldvalue.String("no")
	flag := singleValueFlag("flagKey", expected)

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, err := p.client.JSONVariation(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, expected,
				expectedVariationForSingleValueFlag, defaultVal, noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			sharedtest.UpsertFlag(p.store, &flag)

			actual, detail, err := p.client.JSONVariationDetail(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(expected, expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			assertEvalEvent(t, p.requireSingleEvent(t), flag, evalTestUser, expected,
				expectedVariationForSingleValueFlag, defaultVal, detail.Reason)
		})
	})
}

func TestEvaluatingUnknownFlagReturnsDefault(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		value, err := p.client.StringVariation("flagKey", evalTestUser, "default")
		assert.Error(t, err)
		assert.Equal(t, "default", value)
	})
}

func TestEvaluatingUnknownFlagReturnsDefaultWithDetail(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		_, detail, err := p.client.StringVariationDetail("flagKey", evalTestUser, "default")
		assert.Error(t, err)
		assert.Equal(t, ldvalue.String("default"), detail.Value)
		assert.Equal(t, ldvalue.OptionalInt{}, detail.VariationIndex)
		assert.Equal(t, ldreason.NewEvalReasonError(ldreason.EvalErrorFlagNotFound), detail.Reason)
		assert.True(t, detail.IsDefaultValue())
	})
}

func TestDefaultIsReturnedIfFlagEvaluatesToNil(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").Build() // flag is off and we haven't defined an off variation

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag)

		value, err := p.client.StringVariation(flag.Key, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "default", value)
	})
}

func TestDefaultIsReturnedIfFlagEvaluatesToNilWithDetail(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").Build() // flag is off and we haven't defined an off variation

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag)

		_, detail, err := p.client.StringVariationDetail(flag.Key, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, ldvalue.String("default"), detail.Value)
		assert.Equal(t, ldvalue.OptionalInt{}, detail.VariationIndex)
		assert.Equal(t, ldreason.NewEvalReasonOff(), detail.Reason)
	})
}

func TestDefaultIsReturnedIfFlagReturnsWrongType(t *testing.T) {
	jsonFlag := ldbuilders.NewFlagBuilder("key").SingleVariation(ldvalue.ObjectBuild().Build()).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &jsonFlag)

		v1a, err1a := p.client.BoolVariation(jsonFlag.Key, evalTestUser, false)
		v1b, detail1, err1b := p.client.BoolVariationDetail(jsonFlag.Key, evalTestUser, false)
		assert.NoError(t, err1a)
		assert.NoError(t, err1b)
		assert.False(t, v1a)
		assert.False(t, v1b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail1.Reason.GetErrorKind())

		v2a, err2a := p.client.IntVariation(jsonFlag.Key, evalTestUser, -1)
		v2b, detail2, err2b := p.client.IntVariationDetail(jsonFlag.Key, evalTestUser, -1)
		assert.NoError(t, err2a)
		assert.NoError(t, err2b)
		assert.Equal(t, -1, v2a)
		assert.Equal(t, -1, v2b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail2.Reason.GetErrorKind())

		v3a, err3a := p.client.Float64Variation(jsonFlag.Key, evalTestUser, -1)
		v3b, detail3, err3b := p.client.Float64VariationDetail(jsonFlag.Key, evalTestUser, -1)
		assert.NoError(t, err3a)
		assert.NoError(t, err3b)
		assert.Equal(t, float64(-1), v3a)
		assert.Equal(t, float64(-1), v3b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail3.Reason.GetErrorKind())

		v4a, err4a := p.client.StringVariation(jsonFlag.Key, evalTestUser, "x")
		v4b, detail4, err4b := p.client.StringVariationDetail(jsonFlag.Key, evalTestUser, "x")
		assert.NoError(t, err4a)
		assert.NoError(t, err4b)
		assert.Equal(t, "x", v4a)
		assert.Equal(t, "x", v4b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail4.Reason.GetErrorKind())
	})
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

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag)

		value, err := p.client.StringVariation(flag.Key, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.True(t, e.TrackEvents)
		assert.Equal(t, ldreason.NewEvalReasonRuleMatch(0, "rule-id"), e.Reason)
	})
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

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag)

		value, err := p.client.StringVariation(flag.Key, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.False(t, e.TrackEvents)
		assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
	})
}

func TestEventTrackingAndReasonCanBeForcedForFallthrough(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").
		On(true).
		FallthroughVariation(1).
		Variations(offValue, onValue).
		TrackEventsFallthrough(true).
		Version(1).
		Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag)

		value, err := p.client.StringVariation(flag.Key, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.True(t, e.TrackEvents)
		assert.Equal(t, ldreason.NewEvalReasonFallthrough(), e.Reason)
	})
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfFlagIsNotSet(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("flagKey").
		On(true).
		FallthroughVariation(1).
		Variations(offValue, onValue).
		Version(1).
		Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag)

		value, err := p.client.StringVariation(flag.Key, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.False(t, e.TrackEvents)
		assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
	})
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

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag)

		value, err := p.client.StringVariation(flag.Key, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "off", value)

		e := p.requireSingleEvent(t)
		assert.False(t, e.TrackEvents)
		assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
	})
}

func TestEvaluatingUnknownFlagSendsEvent(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		_, err := p.client.StringVariation("flagKey", evalTestUser, "x")
		assert.Error(t, err)

		e := p.requireSingleEvent(t)
		expectedEvent := ldevents.FeatureRequestEvent{
			BaseEvent: ldevents.BaseEvent{
				CreationDate: e.CreationDate,
				User:         ldevents.User(evalTestUser),
			},
			Key:     "flagKey",
			Value:   ldvalue.String("x"),
			Default: ldvalue.String("x"),
		}
		assert.Equal(t, expectedEvent, e)
	})
}

func TestEvaluatingFlagWithPrerequisiteSendsPrerequisiteEvent(t *testing.T) {
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

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag0)
		sharedtest.UpsertFlag(p.store, &flag1)

		user := lduser.NewUser("userKey")
		_, err := p.client.StringVariation(flag0.Key, user, "x")
		assert.NoError(t, err)

		events := p.events.Events
		assert.Len(t, events, 2)
		e0 := events[0].(ldevents.FeatureRequestEvent)
		expected0 := ldevents.FeatureRequestEvent{
			BaseEvent: ldevents.BaseEvent{
				CreationDate: e0.CreationDate,
				User:         ldevents.User(user),
			},
			Key:       flag1.Key,
			Version:   ldvalue.NewOptionalInt(flag1.Version),
			Value:     ldvalue.String("d"),
			Variation: ldvalue.NewOptionalInt(1),
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
			Version:   ldvalue.NewOptionalInt(flag0.Version),
			Value:     ldvalue.String("b"),
			Variation: ldvalue.NewOptionalInt(1),
			Default:   ldvalue.String("x"),
		}
		assert.Equal(t, expected1, e1)
	})
}

func TestEvalLogsWarningIfUserKeyIsEmpty(t *testing.T) {
	flag := singleValueFlag("flagKey", ldvalue.Bool(true))

	withClientEvalTestParams(func(p clientEvalTestParams) {
		_, _ = p.client.BoolVariation(flag.Key, lduser.NewUser(""), false)
		assert.Len(t, p.mockLog.GetOutput(ldlog.Warn), 1)
		assert.Contains(t, p.mockLog.GetOutput(ldlog.Warn)[0], "User key is blank")
	})
}

func TestEvalErrorIfStoreReturnsError(t *testing.T) {
	myError := errors.New("sorry")
	store := sharedtest.NewCapturingDataStore(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	_ = store.Init(nil)
	store.SetFakeError(myError)
	client := makeTestClientWithConfig(func(c *Config) {
		c.DataStore = sharedtest.SingleDataStoreFactory{Instance: store}
	})
	defer client.Close()

	value, err := client.BoolVariation("flag", evalTestUser, false)
	assert.False(t, value)
	assert.Equal(t, myError, err)
}

func TestEvalErrorIfStoreHasNonFlagObject(t *testing.T) {
	key := "not-really-a-flag"
	notAFlag := 9

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.store.Upsert(datakinds.Features, key,
			ldstoretypes.ItemDescriptor{Version: 1, Item: notAFlag})

		value, err := p.client.BoolVariation(key, evalTestUser, false)
		assert.False(t, value)
		assert.Error(t, err)
	})
}

func TestUnknownFlagErrorLogging(t *testing.T) {
	testEvalErrorLogging(t, nil, "unknown-flag", evalTestUser,
		"unknown feature key: unknown-flag\\. Verify that this feature key exists\\. Returning default value")
}

func TestMalformedFlagErrorLogging(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("bad-flag").On(false).OffVariation(99).Build()
	testEvalErrorLogging(t, &flag, "", evalTestUser,
		"flag evaluation for bad-flag failed with error MALFORMED_FLAG, default value was returned")
}

func testEvalErrorLogging(t *testing.T, flag *ldmodel.FeatureFlag, key string, user lduser.User, expectedMessageRegex string) {
	runTest := func(withLogging bool) {
		mockLoggers := ldlogtest.NewMockLog()
		client := makeTestClientWithConfig(func(c *Config) {
			c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Warn).LogEvaluationErrors(withLogging)
		})
		defer client.Close()
		if flag != nil {
			sharedtest.UpsertFlag(client.store, flag)
			key = flag.Key
		}

		value, _ := client.StringVariation(key, user, "default")
		assert.Equal(t, "default", value)
		if withLogging {
			require.Len(t, mockLoggers.GetAllOutput(), 1)
			assert.Regexp(t, expectedMessageRegex, mockLoggers.GetOutput(ldlog.Warn)[0])
		} else {
			assert.Len(t, mockLoggers.GetAllOutput(), 0)
		}
	}
	runTest(false)
	runTest(true)
}

func TestEvalReturnsDefaultIfClientAndStoreAreNotInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: false},
		}
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	value, err := client.BoolVariation("flagkey", evalTestUser, false)
	require.Error(t, err)
	assert.Equal(t, "feature flag evaluation called before LaunchDarkly client initialization completed",
		err.Error())
	assert.False(t, value)

	assert.Len(t, mockLoggers.GetOutput(ldlog.Warn), 0)
}

func TestEvalUsesStoreAndLogsWarningIfClientIsNotInitializedButStoreIsInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()
	flag := singleValueFlag("flagkey", ldvalue.Bool(true))
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	_ = store.Init(nil)
	_, _ = store.Upsert(datakinds.Features, flag.GetKey(), sharedtest.FlagDescriptor(flag))

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: false},
		}
		c.DataStore = sharedtest.SingleDataStoreFactory{Instance: store}
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	value, err := client.BoolVariation(flag.GetKey(), evalTestUser, false)
	assert.NoError(t, err)
	assert.True(t, value)

	assert.Len(t, mockLoggers.GetOutput(ldlog.Warn), 1)
	assert.Contains(t, mockLoggers.GetOutput(ldlog.Warn)[0], "using last known values")
}

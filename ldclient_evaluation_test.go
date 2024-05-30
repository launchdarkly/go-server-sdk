package ldclient

import (
	gocontext "context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	ldevents "github.com/launchdarkly/go-server-sdk/ldevents/v4"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	evalFlagKey                         = "flag-key"
	expectedVariationForSingleValueFlag = 2
	expectedFlagVersion                 = 1
)

var evalTestUser = lduser.NewUser("userkey")

var fallthroughValue = ldvalue.String("fall")
var offValue = ldvalue.String("off")
var onValue = ldvalue.String("on")

var expectedReasonForSingleValueFlag = ldreason.NewEvalReasonFallthrough()
var noReason = ldreason.EvaluationReason{}

func makeClauseToMatchUser(user ldcontext.Context) ldmodel.Clause {
	return ldbuilders.Clause("key", ldmodel.OperatorIn, ldvalue.String(user.Key()))
}

func makeClauseToNotMatchUser(user ldcontext.Context) ldmodel.Clause {
	return ldbuilders.Clause("key", ldmodel.OperatorIn, ldvalue.String("not-"+user.Key()))
}

type clientEvalTestParams struct {
	client  *LDClient
	store   subsystems.DataStore
	data    *ldtestdata.TestDataSource
	events  *mocks.CapturingEventProcessor
	mockLog *ldlogtest.MockLog
}

func (p clientEvalTestParams) setupSingleValueFlag(key string, value ldvalue.Value) {
	values := []ldvalue.Value{}
	for i := 0; i < expectedVariationForSingleValueFlag; i++ {
		// We add some unused variations so that the result variation index won't be zero, since it's always
		// hard to tell if a zero is an intentional result or just an uninitialized variable.
		values = append(values, ldvalue.String("should not get this value"))
	}
	values = append(values, value)
	p.data.Update(p.data.Flag(key).On(true).
		FallthroughVariationIndex(expectedVariationForSingleValueFlag).
		Variations(values...))
}

func withClientEvalTestParams(callback func(clientEvalTestParams)) {
	p := clientEvalTestParams{}
	p.store = datastore.NewInMemoryDataStore(ldlog.NewDisabledLoggers())
	p.data = ldtestdata.DataSource()
	p.events = &mocks.CapturingEventProcessor{}
	p.mockLog = ldlogtest.NewMockLog()
	config := Config{
		Offline:    false,
		DataStore:  mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: p.store},
		DataSource: p.data,
		Events:     mocks.SingleComponentConfigurer[ldevents.EventProcessor]{Instance: p.events},
		Logging:    ldcomponents.Logging().Loggers(p.mockLog.Loggers),
	}
	p.client, _ = MakeCustomClient("sdk_key", config, 0)
	defer p.client.Close()
	callback(p)
}

func (p clientEvalTestParams) requireSingleEvent(t *testing.T) ldevents.EvaluationData {
	events := p.events.Events
	require.Equal(t, 1, len(events))
	return events[0].(ldevents.EvaluationData)
}

func (p clientEvalTestParams) expectSingleEvaluationEvent(
	t *testing.T,
	flagKey string,
	value ldvalue.Value,
	defaultVal ldvalue.Value,
	reason ldreason.EvaluationReason,
) {
	assertEvalEvent(t, p.requireSingleEvent(t), flagKey, expectedFlagVersion, evalTestUser, value,
		expectedVariationForSingleValueFlag, defaultVal, reason)
}

func assertEvalEvent(
	t *testing.T,
	actualEvent ldevents.EvaluationData,
	flagKey string,
	flagVersion int,
	user ldcontext.Context,
	value ldvalue.Value,
	variation int,
	defaultVal ldvalue.Value,
	reason ldreason.EvaluationReason,
) {
	expectedEvent := ldevents.EvaluationData{
		BaseEvent: ldevents.BaseEvent{
			CreationDate: actualEvent.CreationDate,
			Context:      ldevents.Context(user),
		},
		Key:       flagKey,
		Version:   ldvalue.NewOptionalInt(flagVersion),
		Value:     value,
		Variation: ldvalue.NewOptionalInt(variation),
		Default:   defaultVal,
		Reason:    reason,
	}
	assert.Equal(t, expectedEvent, actualEvent)
}

func TestExcludeFromSummaries(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key1").On(true).ExcludeFromSummaries(true).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").On(true).AddPrerequisite(flag1.Key, 0).Build()

	t.Run("flag can be excluded from summaries", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(flag1)
			p.data.UsePreconfiguredFlag(flag2)

			_, err := p.client.BoolVariation(flag1.Key, evalTestUser, false)

			assert.NoError(t, err)
			event := p.requireSingleEvent(t)

			assert.False(t, event.SamplingRatio.IsDefined())
			assert.True(t, event.ExcludeFromSummaries)
		})
	})

	t.Run("prereq can be excluded individually", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(flag1)
			p.data.UsePreconfiguredFlag(flag2)

			_, err := p.client.BoolVariation(flag2.Key, evalTestUser, false)

			assert.NoError(t, err)
			events := p.events.Events

			assert.Len(t, events, 2)

			assert.True(t, events[0].(ldevents.EvaluationData).ExcludeFromSummaries)
			assert.Equal(t, flag1.Key, events[0].(ldevents.EvaluationData).Key)

			assert.False(t, events[1].(ldevents.EvaluationData).ExcludeFromSummaries)
			assert.Equal(t, flag2.Key, events[1].(ldevents.EvaluationData).Key)
		})
	})
}

func TestBoolVariation(t *testing.T) {
	expected, defaultVal := true, false

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, err := p.client.BoolVariation(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), noReason)
		})
	})

	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, err := p.client.BoolVariationCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, detail, err := p.client.BoolVariationDetail(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Bool(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), detail.Reason)
		})
	})

	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Bool(true))

			actual, detail, err := p.client.BoolVariationDetailCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Bool(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Bool(expected), ldvalue.Bool(defaultVal), detail.Reason)
		})
	})
}

func TestIntVariation(t *testing.T) {
	expected, defaultVal := 100, 10000

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

			actual, err := p.client.IntVariation(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

			actual, detail, err := p.client.IntVariationDetail(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Int(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), detail.Reason)
		})

		t.Run("simpleCtx", func(t *testing.T) {
			withClientEvalTestParams(func(p clientEvalTestParams) {
				p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

				actual, err := p.client.IntVariationCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

				assert.NoError(t, err)
				assert.Equal(t, expected, actual)

				p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), noReason)
			})
		})

		t.Run("detailCtx", func(t *testing.T) {
			withClientEvalTestParams(func(p clientEvalTestParams) {
				p.setupSingleValueFlag(evalFlagKey, ldvalue.Int(expected))

				actual, detail, err := p.client.IntVariationDetailCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

				assert.NoError(t, err)
				assert.Equal(t, expected, actual)
				assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Int(expected), expectedVariationForSingleValueFlag,
					expectedReasonForSingleValueFlag), detail)

				p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Int(expected), ldvalue.Int(defaultVal), detail.Reason)
			})
		})
	})

	t.Run("rounds float toward zero", func(t *testing.T) {
		flag1Key, flag2Key, flag3Key, flag4Key := "flag1", "flag2", "flag3", "flag4"
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(flag1Key, ldvalue.Float64(2.25))
			p.setupSingleValueFlag(flag2Key, ldvalue.Float64(2.75))
			p.setupSingleValueFlag(flag3Key, ldvalue.Float64(-2.25))
			p.setupSingleValueFlag(flag4Key, ldvalue.Float64(-2.75))

			actual, err := p.client.IntVariation(flag1Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, 2, actual)

			actual, err = p.client.IntVariation(flag2Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, 2, actual)

			actual, err = p.client.IntVariation(flag3Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, -2, actual)

			actual, err = p.client.IntVariation(flag4Key, evalTestUser, 0)
			assert.NoError(t, err)
			assert.Equal(t, -2, actual)
		})
	})
}

func TestFloat64Variation(t *testing.T) {
	expected, defaultVal := 100.01, 0.0

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, err := p.client.Float64Variation(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, detail, err := p.client.Float64VariationDetail(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Float64(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), detail.Reason)
		})
	})

	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, err := p.client.Float64VariationCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), noReason)
		})
	})

	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.Float64(expected))

			actual, detail, err := p.client.Float64VariationDetailCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Float64(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.Float64(expected), ldvalue.Float64(defaultVal), detail.Reason)
		})
	})
}

func TestStringVariation(t *testing.T) {
	expected, defaultVal := "b", "a"

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, err := p.client.StringVariation(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), noReason)
		})
	})

	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, err := p.client.StringVariationCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), noReason)
		})
	})

	t.Run("sampling ratios are not defined by default", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			_, err := p.client.StringVariation(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)

			events := p.events.Events
			assert.Len(t, events, 1)

			eval := events[0]
			assert.False(t, eval.(ldevents.EvaluationData).SamplingRatio.IsDefined())
		})
	})

	t.Run("sampling ratios can be defined", func(t *testing.T) {
		flag := ldbuilders.NewFlagBuilder("flag").
			On(true).
			SamplingRatio(21).
			Build()
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(flag)

			_, err := p.client.StringVariation(flag.Key, evalTestUser, defaultVal)

			assert.NoError(t, err)

			events := p.events.Events
			assert.Len(t, events, 1)

			eval := events[0]
			assert.Equal(t, 21, eval.(ldevents.EvaluationData).SamplingRatio.IntValue())
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, detail, err := p.client.StringVariationDetail(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.String(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), detail.Reason)
		})
	})

	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.String(expected))

			actual, detail, err := p.client.StringVariationDetailCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.String(expected), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.String(expected), ldvalue.String(defaultVal), detail.Reason)
		})
	})
}

func TestJSONRawVariation(t *testing.T) {
	expectedValue := map[string]interface{}{"field2": "value2"}
	expectedJSON, _ := json.Marshal(expectedValue)
	expectedRaw := json.RawMessage(expectedJSON)
	defaultVal := json.RawMessage([]byte(`{"default":"default"}`))

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue))

			actual, err := p.client.JSONVariation(evalFlagKey, evalTestUser, ldvalue.Raw(defaultVal))

			assert.NoError(t, err)
			assert.Equal(t, expectedRaw, actual.AsRaw())

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue),
				ldvalue.CopyArbitraryValue(defaultVal), noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue))

			actual, detail, err := p.client.JSONVariationDetail(evalFlagKey, evalTestUser, ldvalue.Raw(defaultVal))

			assert.NoError(t, err)
			assert.Equal(t, expectedRaw, actual.AsRaw())
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Parse(expectedRaw), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue),
				ldvalue.CopyArbitraryValue(defaultVal), detail.Reason)
		})
	})

	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue))

			actual, err := p.client.JSONVariationCtx(gocontext.TODO(), evalFlagKey, evalTestUser, ldvalue.Raw(defaultVal))

			assert.NoError(t, err)
			assert.Equal(t, expectedRaw, actual.AsRaw())

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue),
				ldvalue.CopyArbitraryValue(defaultVal), noReason)
		})
	})

	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue))

			actual, detail, err := p.client.JSONVariationDetailCtx(gocontext.TODO(), evalFlagKey, evalTestUser, ldvalue.Raw(defaultVal))

			assert.NoError(t, err)
			assert.Equal(t, expectedRaw, actual.AsRaw())
			assert.Equal(t, ldreason.NewEvaluationDetail(ldvalue.Parse(expectedRaw), expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, ldvalue.CopyArbitraryValue(expectedValue),
				ldvalue.CopyArbitraryValue(defaultVal), detail.Reason)
		})
	})
}

func TestJSONVariation(t *testing.T) {
	expected := ldvalue.CopyArbitraryValue(map[string]interface{}{"field2": "value2"})
	defaultVal := ldvalue.String("no")

	t.Run("simple", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, err := p.client.JSONVariation(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, noReason)
		})
	})

	t.Run("detail", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, detail, err := p.client.JSONVariationDetail(evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(expected, expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, detail.Reason)
		})
	})

	t.Run("simpleCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, err := p.client.JSONVariationCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, noReason)
		})
	})

	t.Run("detailCtx", func(t *testing.T) {
		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.setupSingleValueFlag(evalFlagKey, expected)

			actual, detail, err := p.client.JSONVariationDetailCtx(gocontext.TODO(), evalFlagKey, evalTestUser, defaultVal)

			assert.NoError(t, err)
			assert.Equal(t, expected, actual)
			assert.Equal(t, ldreason.NewEvaluationDetail(expected, expectedVariationForSingleValueFlag,
				expectedReasonForSingleValueFlag), detail)

			p.expectSingleEvaluationEvent(t, evalFlagKey, expected, defaultVal, detail.Reason)
		})
	})
}

func TestEvaluatingUnknownFlagReturnsDefault(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		value, err := p.client.StringVariation("no-such-flag", evalTestUser, "default")
		assert.Error(t, err)
		assert.Equal(t, "default", value)
	})
}

func TestEvaluatingUnknownFlagReturnsDefaultWithDetail(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		_, detail, err := p.client.StringVariationDetail("no-such-flag", evalTestUser, "default")
		assert.Error(t, err)
		assert.Equal(t, ldvalue.String("default"), detail.Value)
		assert.Equal(t, ldvalue.OptionalInt{}, detail.VariationIndex)
		assert.Equal(t, ldreason.NewEvalReasonError(ldreason.EvalErrorFlagNotFound), detail.Reason)
		assert.True(t, detail.IsDefaultValue())
	})
}

func TestDefaultIsReturnedIfFlagEvaluatesToNil(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).Build() // flag is off and we haven't defined an off variation

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag)

		value, err := p.client.StringVariation(evalFlagKey, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "default", value)
	})
}

func TestDefaultIsReturnedIfFlagEvaluatesToNilWithDetail(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).Build() // flag is off and we haven't defined an off variation

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag)

		_, detail, err := p.client.StringVariationDetail(evalFlagKey, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, ldvalue.String("default"), detail.Value)
		assert.Equal(t, ldvalue.OptionalInt{}, detail.VariationIndex)
		assert.Equal(t, ldreason.NewEvalReasonOff(), detail.Reason)
	})
}

func TestDefaultIsReturnedIfFlagReturnsWrongType(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.setupSingleValueFlag(evalFlagKey, ldvalue.ArrayOf())

		v1a, err1a := p.client.BoolVariation(evalFlagKey, evalTestUser, false)
		v1b, detail1, err1b := p.client.BoolVariationDetail(evalFlagKey, evalTestUser, false)
		assert.NoError(t, err1a)
		assert.NoError(t, err1b)
		assert.False(t, v1a)
		assert.False(t, v1b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail1.Reason.GetErrorKind())

		v2a, err2a := p.client.IntVariation(evalFlagKey, evalTestUser, -1)
		v2b, detail2, err2b := p.client.IntVariationDetail(evalFlagKey, evalTestUser, -1)
		assert.NoError(t, err2a)
		assert.NoError(t, err2b)
		assert.Equal(t, -1, v2a)
		assert.Equal(t, -1, v2b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail2.Reason.GetErrorKind())

		v3a, err3a := p.client.Float64Variation(evalFlagKey, evalTestUser, -1)
		v3b, detail3, err3b := p.client.Float64VariationDetail(evalFlagKey, evalTestUser, -1)
		assert.NoError(t, err3a)
		assert.NoError(t, err3b)
		assert.Equal(t, float64(-1), v3a)
		assert.Equal(t, float64(-1), v3b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail3.Reason.GetErrorKind())

		v4a, err4a := p.client.StringVariation(evalFlagKey, evalTestUser, "x")
		v4b, detail4, err4b := p.client.StringVariationDetail(evalFlagKey, evalTestUser, "x")
		assert.NoError(t, err4a)
		assert.NoError(t, err4b)
		assert.Equal(t, "x", v4a)
		assert.Equal(t, "x", v4b)
		assert.Equal(t, ldreason.EvalErrorWrongType, detail4.Reason.GetErrorKind())
	})
}

func TestEvaluateWithInvalidContext(t *testing.T) {
	flagKey := "flag"
	for _, contextParams := range []struct {
		name      string
		context   ldcontext.Context
		errorText string
	}{
		{"empty key", ldcontext.New(""), "context key must not be empty"},
		{"invalid kind", ldcontext.NewWithKind("!bad!", "key"), "context kind contains disallowed characters"},
	} {
		t.Run(contextParams.name, func(t *testing.T) {
			c := contextParams.context
			for _, evalFnParams := range []struct {
				name string
				fn   func(*LDClient) error
			}{
				{"BoolVariation", func(client *LDClient) error { _, err := client.BoolVariation(flagKey, c, false); return err }},
				{"IntVariation", func(client *LDClient) error { _, err := client.IntVariation(flagKey, c, 0); return err }},
				{"Float64Variation", func(client *LDClient) error { _, err := client.Float64Variation(flagKey, c, 0); return err }},
				{"StringVariation", func(client *LDClient) error { _, err := client.StringVariation(flagKey, c, ""); return err }},
				{"JSONVariation", func(client *LDClient) error { _, err := client.JSONVariation(flagKey, c, ldvalue.Null()); return err }},
			} {
				t.Run(evalFnParams.name, func(t *testing.T) {
					withClientEvalTestParams(func(p clientEvalTestParams) {
						err := evalFnParams.fn(p.client)
						assert.Error(t, err)
						p.mockLog.AssertMessageMatch(t, true, ldlog.Warn, contextParams.errorText)
					})
				})
			}
			for _, evalFnParams := range []struct {
				name string
				fn   func(*LDClient) (ldreason.EvaluationDetail, error)
			}{
				{"BoolVariationDetail",
					func(client *LDClient) (ldreason.EvaluationDetail, error) {
						_, detail, err := client.BoolVariationDetail(flagKey, c, false)
						return detail, err
					}},
				{"IntVariationDetail",
					func(client *LDClient) (ldreason.EvaluationDetail, error) {
						_, detail, err := client.IntVariationDetail(flagKey, c, 0)
						return detail, err
					}},
				{"Float64VariationDetail",
					func(client *LDClient) (ldreason.EvaluationDetail, error) {
						_, detail, err := client.Float64VariationDetail(flagKey, c, 0)
						return detail, err
					}},
				{"StringVariationDetail", func(client *LDClient) (ldreason.EvaluationDetail, error) {
					_, detail, err := client.StringVariationDetail(flagKey, c, "")
					return detail, err
				}},
				{"JSONVariationDetail", func(client *LDClient) (ldreason.EvaluationDetail, error) {
					_, detail, err := client.JSONVariationDetail(flagKey, c, ldvalue.Null())
					return detail, err
				}},
			} {
				t.Run(evalFnParams.name, func(t *testing.T) {
					withClientEvalTestParams(func(p clientEvalTestParams) {
						detail, err := evalFnParams.fn(p.client)
						assert.Error(t, err)
						assert.Equal(t, ldreason.NewEvalReasonError(ldreason.EvalErrorUserNotSpecified), detail.Reason)
						p.mockLog.AssertMessageMatch(t, true, ldlog.Warn, contextParams.errorText)
					})
				})
			}
		})
	}
}

func TestEventTrackingAndReasonCanBeForcedForRule(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).
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
		p.data.UsePreconfiguredFlag(flag)

		value, err := p.client.StringVariation(evalFlagKey, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.True(t, e.RequireFullEvent)
		assert.Equal(t, ldreason.NewEvalReasonRuleMatch(0, "rule-id"), e.Reason)
	})
}

func TestEventTrackingAndReasonAreNotForcedIfFlagIsNotSetForMatchingRule(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).
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
		p.data.UsePreconfiguredFlag(flag)

		value, err := p.client.StringVariation(evalFlagKey, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.False(t, e.RequireFullEvent)
		assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
	})
}

func TestEventTrackingAndReasonCanBeForcedForFallthrough(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).
		On(true).
		FallthroughVariation(1).
		Variations(offValue, onValue).
		TrackEventsFallthrough(true).
		Version(1).
		Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag)

		value, err := p.client.StringVariation(evalFlagKey, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.True(t, e.RequireFullEvent)
		assert.Equal(t, ldreason.NewEvalReasonFallthrough(), e.Reason)
	})
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfFlagIsNotSet(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).
		On(true).
		FallthroughVariation(1).
		Variations(offValue, onValue).
		Version(1).
		Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag)

		value, err := p.client.StringVariation(evalFlagKey, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "on", value)

		e := p.requireSingleEvent(t)
		assert.False(t, e.RequireFullEvent)
		assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
	})
}

func TestEventTrackingAndReasonAreNotForcedForFallthroughIfReasonIsNotFallthrough(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.Update(p.data.Flag(evalFlagKey).Variations(offValue, onValue).OffVariationIndex(0).On(false))

		value, err := p.client.StringVariation(evalFlagKey, evalTestUser, "default")
		assert.NoError(t, err)
		assert.Equal(t, "off", value)

		e := p.requireSingleEvent(t)
		assert.False(t, e.RequireFullEvent)
		assert.Equal(t, ldreason.EvaluationReason{}, e.Reason)
	})
}

func TestEvaluatingUnknownFlagSendsEvent(t *testing.T) {
	withClientEvalTestParams(func(p clientEvalTestParams) {
		_, err := p.client.StringVariation("no-such-flag", evalTestUser, "x")
		assert.Error(t, err)

		e := p.requireSingleEvent(t)
		expectedEvent := ldevents.EvaluationData{
			BaseEvent: ldevents.BaseEvent{
				CreationDate: e.CreationDate,
				Context:      ldevents.Context(evalTestUser),
			},
			Key:     "no-such-flag",
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
		p.data.UsePreconfiguredFlag(flag0)
		p.data.UsePreconfiguredFlag(flag1)

		user := lduser.NewUser("userKey")
		_, err := p.client.StringVariation(flag0.Key, user, "x")
		assert.NoError(t, err)

		events := p.events.Events
		assert.Len(t, events, 2)
		e0 := events[0].(ldevents.EvaluationData)
		expected0 := ldevents.EvaluationData{
			BaseEvent: ldevents.BaseEvent{
				CreationDate: e0.CreationDate,
				Context:      ldevents.Context(user),
			},
			Key:       flag1.Key,
			Version:   ldvalue.NewOptionalInt(1),
			Value:     ldvalue.String("d"),
			Variation: ldvalue.NewOptionalInt(1),
			Default:   ldvalue.Null(),
			PrereqOf:  ldvalue.NewOptionalString(flag0.Key),
		}
		assert.Equal(t, expected0, e0)

		e1 := events[1].(ldevents.EvaluationData)
		expected1 := ldevents.EvaluationData{
			BaseEvent: ldevents.BaseEvent{
				CreationDate: e1.CreationDate,
				Context:      ldevents.Context(user),
			},
			Key:       flag0.Key,
			Version:   ldvalue.NewOptionalInt(1),
			Value:     ldvalue.String("b"),
			Variation: ldvalue.NewOptionalInt(1),
			Default:   ldvalue.String("x"),
		}
		assert.Equal(t, expected1, e1)
	})
}

func TestEvalErrorIfStoreReturnsError(t *testing.T) {
	myError := errors.New("sorry")
	store := mocks.NewCapturingDataStore(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	_ = store.Init(nil)
	store.SetFakeError(myError)
	client := makeTestClientWithConfig(func(c *Config) {
		c.DataStore = mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store}
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
		"",
		"unknown feature key: unknown-flag\\. Verify that this feature key exists\\. Returning default value")
}

func TestMalformedFlagErrorLogging(t *testing.T) {
	flag := ldbuilders.NewFlagBuilder("bad-flag").On(false).OffVariation(99).Build()
	testEvalErrorLogging(t, &flag, "", evalTestUser,
		`Invalid flag configuration.*"bad-flag".*nonexistent variation index 99`,
		"Flag evaluation for bad-flag failed with error MALFORMED_FLAG, default value was returned")
}

func testEvalErrorLogging(t *testing.T, flag *ldmodel.FeatureFlag, key string, user ldcontext.Context,
	expectedErrorRegex, expectedWarningRegex string) {
	runTest := func(withLogging bool) {
		mockLoggers := ldlogtest.NewMockLog()
		testData := ldtestdata.DataSource()
		client := makeTestClientWithConfig(func(c *Config) {
			c.DataSource = testData
			c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers).MinLevel(ldlog.Warn).LogEvaluationErrors(withLogging)
		})
		defer client.Close()
		if flag != nil {
			testData.UsePreconfiguredFlag(*flag)
			key = flag.Key
		}

		value, _ := client.StringVariation(key, user, "default")
		assert.Equal(t, "default", value)

		if expectedErrorRegex == "" {
			require.Len(t, mockLoggers.GetOutput(ldlog.Error), 0)
		} else {
			require.Len(t, mockLoggers.GetOutput(ldlog.Error), 1)
			assert.Regexp(t, expectedErrorRegex, mockLoggers.GetOutput(ldlog.Error)[0])
		}

		if withLogging {
			require.Len(t, mockLoggers.GetOutput(ldlog.Warn), 1)
			assert.Regexp(t, expectedWarningRegex, mockLoggers.GetOutput(ldlog.Warn)[0])
		} else {
			assert.Len(t, mockLoggers.GetOutput(ldlog.Warn), 0)
		}
	}
	runTest(false)
	runTest(true)
}

func TestEvalReturnsDefaultIfClientAndStoreAreNotInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = mocks.DataSourceThatNeverInitializes()
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
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).SingleVariation(ldvalue.Bool(true)).Build()
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	_ = store.Init(nil)
	_, _ = store.Upsert(datakinds.Features, flag.Key, sharedtest.FlagDescriptor(flag))

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = mocks.DataSourceThatNeverInitializes()
		c.DataStore = mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store}
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	value, err := client.BoolVariation(flag.Key, evalTestUser, false)
	assert.NoError(t, err)
	assert.True(t, value)

	assert.Len(t, mockLoggers.GetOutput(ldlog.Warn), 1)
	assert.Contains(t, mockLoggers.GetOutput(ldlog.Warn)[0], "using last known values")
}

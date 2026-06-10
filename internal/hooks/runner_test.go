package hooks

import (
	gocontext "context"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
)

type mockEnvironmentIDProvider struct {
	environmentID ldvalue.OptionalString
}

func (e mockEnvironmentIDProvider) GetEnvironmentID() ldvalue.OptionalString {
	return e.environmentID
}

func newMockEnvironmentIDProvider(environmentID string) mockEnvironmentIDProvider {
	return mockEnvironmentIDProvider{
		environmentID: ldvalue.NewOptionalString(environmentID),
	}
}

func TestHookRunner(t *testing.T) {
	falseValue := ldvalue.Bool(false)
	ldContext := ldcontext.New("test-context")
	flagKey := "test-flag"
	testMethod := "testMethod"
	defaultDetail := ldreason.NewEvaluationDetail(falseValue, 0, ldreason.NewEvalReasonFallthrough())
	eventKey := "test-event"
	basicEvalResult := func() (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
		return defaultDetail, nil, nil
	}

	t.Run("with no hooks", func(t *testing.T) {
		runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{}, mockEnvironmentIDProvider{})

		t.Run("prepare evaluation series", func(t *testing.T) {
			res := runner.prepareEvaluationSeries(flagKey, ldContext, falseValue, testMethod)
			emptyEvaluationExecutionAssertions(t, res, ldContext)
		})

		t.Run("run evaluation execution", func(t *testing.T) {
			detail, flag, err := runner.RunEvaluation(
				gocontext.Background(),
				flagKey,
				ldContext,
				falseValue,
				testMethod,
				basicEvalResult,
			)
			assert.Equal(t, defaultDetail, detail)
			assert.Nil(t, flag)
			assert.Nil(t, err)
		})

		// RunTrack has no return values so there's nothing to test
	})

	t.Run("verify evaluation execution and order", func(t *testing.T) {
		tracker := newOrderTracker()
		hookA := createOrderTrackingEvalHook("a", tracker)
		hookB := createOrderTrackingEvalHook("b", tracker)
		runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB}, newMockEnvironmentIDProvider("env-id"))

		_, _, _ = runner.RunEvaluation(
			gocontext.Background(),
			flagKey,
			ldContext,
			falseValue,
			testMethod,
			basicEvalResult,
		)

		hookA.Verify(t, sharedtest.HookExpectedCall{
			HookStage: sharedtest.HookStageBeforeEvaluation,
			EvalCapture: sharedtest.HookEvalCapture{
				EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext(
					flagKey,
					ldContext,
					falseValue,
					testMethod,
					ldvalue.NewOptionalString("env-id"),
				),
				EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
				GoContext:            gocontext.Background(),
			},
		},
			sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageAfterEvaluation,
				EvalCapture: sharedtest.HookEvalCapture{
					EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext(
						flagKey,
						ldContext,
						falseValue,
						testMethod,
						ldvalue.NewOptionalString("env-id"),
					),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
					GoContext:            gocontext.Background(),
					EvaluationDetail:     defaultDetail,
				},
			})

		hookB.Verify(t, sharedtest.HookExpectedCall{
			HookStage: sharedtest.HookStageBeforeEvaluation,
			EvalCapture: sharedtest.HookEvalCapture{
				EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext(
					flagKey,
					ldContext,
					falseValue,
					testMethod,
					ldvalue.NewOptionalString("env-id"),
				),
				EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
				GoContext:            gocontext.Background(),
			}},
			sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageAfterEvaluation,
				EvalCapture: sharedtest.HookEvalCapture{
					EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext(
						flagKey,
						ldContext,
						falseValue,
						testMethod,
						ldvalue.NewOptionalString("env-id"),
					),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
					GoContext:            gocontext.Background(),
					EvaluationDetail:     defaultDetail,
				},
			})

		// BeforeEvaluation should execute in registration order.
		assert.Equal(t, []string{"a", "b"}, tracker.orderBefore)
		// AfterEvaluation should execute in reverse registration order.
		assert.Equal(t, []string{"b", "a"}, tracker.orderAfter)
	})

	t.Run("verify track execution and order", func(t *testing.T) {
		tracker := newOrderTracker()
		hookA := createOrderTrackingTrackHook("a", tracker)
		hookB := createOrderTrackingTrackHook("b", tracker)
		runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB}, newMockEnvironmentIDProvider("env-id"))

		runner.RunTrack(gocontext.Background(), eventKey, ldContext, nil, ldvalue.Null(), func() {})

		hookA.Verify(t, sharedtest.HookExpectedCall{
			HookStage: sharedtest.HookStageAfterTrack,
			TrackCapture: sharedtest.HookTrackCapture{
				TrackSeriesContext: ldhooks.NewTrackSeriesContext(ldContext, eventKey, nil, ldvalue.Null(), ldvalue.NewOptionalString("env-id")),
				GoContext:          gocontext.Background(),
			},
		})

		hookB.Verify(t, sharedtest.HookExpectedCall{
			HookStage: sharedtest.HookStageAfterTrack,
			TrackCapture: sharedtest.HookTrackCapture{
				TrackSeriesContext: ldhooks.NewTrackSeriesContext(ldContext, eventKey, nil, ldvalue.Null(), ldvalue.NewOptionalString("env-id")),
				GoContext:          gocontext.Background(),
			},
		})

		// AfterTrack should execute in registration order.
		assert.Equal(t, []string{"a", "b"}, tracker.orderAfter)
	})
}

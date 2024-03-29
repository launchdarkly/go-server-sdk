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

func TestHookRunner(t *testing.T) {
	falseValue := ldvalue.Bool(false)
	ldContext := ldcontext.New("test-context")
	flagKey := "test-flag"
	testMethod := "testMethod"
	defaultDetail := ldreason.NewEvaluationDetail(falseValue, 0, ldreason.NewEvalReasonFallthrough())
	basicResult := func() (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
		return defaultDetail, nil, nil
	}

	t.Run("with no hooks", func(t *testing.T) {
		runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{})

		t.Run("prepare evaluation series", func(t *testing.T) {
			res := runner.prepareEvaluationSeries(flagKey, ldContext, falseValue, testMethod)
			emptyExecutionAssertions(t, res, ldContext)
		})

		t.Run("run execution", func(t *testing.T) {
			detail, flag, err := runner.RunEvaluation(
				gocontext.Background(),
				flagKey,
				ldContext,
				falseValue,
				testMethod,
				basicResult,
			)
			assert.Equal(t, defaultDetail, detail)
			assert.Nil(t, flag)
			assert.Nil(t, err)
		})
	})

	t.Run("verify execution and order", func(t *testing.T) {
		tracker := newOrderTracker()
		hookA := createOrderTrackingHook("a", tracker)
		hookB := createOrderTrackingHook("b", tracker)
		runner := NewRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

		_, _, _ = runner.RunEvaluation(
			gocontext.Background(),
			flagKey,
			ldContext,
			falseValue,
			testMethod,
			basicResult,
		)

		hookA.Verify(t, sharedtest.HookExpectedCall{
			HookStage: sharedtest.HookStageBeforeEvaluation,
			EvalCapture: sharedtest.HookEvalCapture{
				EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext(
					flagKey,
					ldContext,
					falseValue,
					testMethod,
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
					),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
					GoContext:            gocontext.Background(),
					Detail:               defaultDetail,
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
					),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
					GoContext:            gocontext.Background(),
					Detail:               defaultDetail,
				},
			})

		// BeforeEvaluation should execute in registration order.
		assert.Equal(t, []string{"a", "b"}, tracker.orderBefore)
		assert.Equal(t, []string{"b", "a"}, tracker.orderAfter)
	})
}

package ldclient

import (
	"context"
	"errors"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
)

func emptyExecutionAssertions(t *testing.T, res evaluationExecution, ldContext ldcontext.Context) {
	assert.Empty(t, res.hooks)
	assert.Empty(t, res.data)
	assert.Equal(t, ldContext, res.context.Context())
	assert.Equal(t, "test-flag", res.context.FlagKey())
	assert.Equal(t, "testMethod", res.context.Method())
	assert.Equal(t, ldvalue.Bool(false), res.context.DefaultValue())
}

func TestHookRunner(t *testing.T) {
	t.Run("with no hooks", func(t *testing.T) {
		runner := newHookRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{})

		t.Run("prepare evaluation series", func(t *testing.T) {
			ldContext := ldcontext.New("test-context")
			res := runner.prepareEvaluationSeries("test-flag", ldContext, false, "testMethod")
			emptyExecutionAssertions(t, res, ldContext)
		})

		t.Run("run before evaluation", func(t *testing.T) {
			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")
			res := runner.beforeEvaluation(context.Background(), execution)
			emptyExecutionAssertions(t, res, ldContext)
		})

		t.Run("run after evaluation", func(t *testing.T) {
			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")
			res := runner.afterEvaluation(context.Background(), execution,
				ldreason.NewEvaluationDetail(ldvalue.Bool(false), 0,
					ldreason.NewEvalReasonFallthrough()))
			emptyExecutionAssertions(t, res, ldContext)
		})
	})

	t.Run("with hooks", func(t *testing.T) {
		t.Run("prepare evaluation series", func(t *testing.T) {
			hookA := sharedtest.NewTestHook("a")
			hookB := sharedtest.NewTestHook("b")
			runner := newHookRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

			ldContext := ldcontext.New("test-context")
			res := runner.prepareEvaluationSeries("test-flag", ldContext, false, "testMethod")

			assert.Len(t, res.hooks, 2)
			assert.Len(t, res.data, 2)
			assert.Equal(t, ldContext, res.context.Context())
			assert.Equal(t, "test-flag", res.context.FlagKey())
			assert.Equal(t, "testMethod", res.context.Method())
			assert.Equal(t, ldvalue.Bool(false), res.context.DefaultValue())
			assert.Equal(t, res.data[0], ldhooks.EmptyEvaluationSeriesData())
			assert.Equal(t, res.data[1], ldhooks.EmptyEvaluationSeriesData())
		})

		t.Run("run before evaluation", func(t *testing.T) {
			orderBefore := make([]string, 0)
			hookA := sharedtest.NewTestHook("a")
			hookA.BeforeInject = func(
				ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
			) (ldhooks.EvaluationSeriesData, error) {
				orderBefore = append(orderBefore, "a")
				return data, nil
			}
			hookB := sharedtest.NewTestHook("b")
			hookB.BeforeInject = func(ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
			) (ldhooks.EvaluationSeriesData, error) {
				orderBefore = append(orderBefore, "b")
				return data, nil
			}
			runner := newHookRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")
			_ = runner.beforeEvaluation(context.Background(), execution)

			hookA.Expect(t, sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageBeforeEvaluation,
				EvalCapture: sharedtest.HookEvalCapture{
					EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
						false, "testMethod"),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
				}})

			hookB.Expect(t, sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageBeforeEvaluation,
				EvalCapture: sharedtest.HookEvalCapture{
					EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
						false, "testMethod"),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
				}})

			// BeforeEvaluation should execute in registration order.
			assert.Equal(t, []string{"a", "b"}, orderBefore)
		})

		t.Run("run after evaluation", func(t *testing.T) {
			orderAfter := make([]string, 0)
			hookA := sharedtest.NewTestHook("a")
			hookA.AfterInject = func(
				ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
				detail ldreason.EvaluationDetail,
			) (ldhooks.EvaluationSeriesData, error) {
				orderAfter = append(orderAfter, "a")
				return data, nil
			}
			hookB := sharedtest.NewTestHook("b")
			hookB.AfterInject = func(
				ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
				detail ldreason.EvaluationDetail,
			) (ldhooks.EvaluationSeriesData, error) {
				orderAfter = append(orderAfter, "b")
				return data, nil
			}
			runner := newHookRunner(sharedtest.NewTestLoggers(), []ldhooks.Hook{hookA, hookB})

			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")
			detail := ldreason.NewEvaluationDetail(ldvalue.Bool(false), 0,
				ldreason.NewEvalReasonFallthrough())
			_ = runner.afterEvaluation(context.Background(), execution, detail)

			hookA.Expect(t, sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageAfterEvaluation,
				EvalCapture: sharedtest.HookEvalCapture{
					EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
						false, "testMethod"),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
					Detail:               detail,
				}})

			hookB.Expect(t, sharedtest.HookExpectedCall{
				HookStage: sharedtest.HookStageAfterEvaluation,
				EvalCapture: sharedtest.HookEvalCapture{
					EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", ldContext,
						false, "testMethod"),
					EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
					Detail:               detail,
				}})

			// AfterEvaluation should execute in reverse registration order.
			assert.Equal(t, []string{"b", "a"}, orderAfter)
		})

		t.Run("run before evaluation with an error", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			hookA := sharedtest.NewTestHook("a")
			hookA.BeforeInject = func(
				ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
			) (ldhooks.EvaluationSeriesData, error) {
				return ldhooks.NewEvaluationSeriesBuilder(data).
					Set("testA", "A").
					Build(), errors.New("something bad")
			}
			hookB := sharedtest.NewTestHook("b")
			hookB.BeforeInject = func(
				ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
			) (ldhooks.EvaluationSeriesData, error) {
				return ldhooks.NewEvaluationSeriesBuilder(data).
					Set("testB", "testB").
					Build(), nil
			}

			runner := newHookRunner(mockLog.Loggers, []ldhooks.Hook{hookA, hookB})
			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")

			res := runner.beforeEvaluation(context.Background(), execution)
			assert.Len(t, res.hooks, 2)
			assert.Len(t, res.data, 2)
			assert.Equal(t, ldContext, res.context.Context())
			assert.Equal(t, "test-flag", res.context.FlagKey())
			assert.Equal(t, "testMethod", res.context.Method())
			assert.Equal(t, ldhooks.EmptyEvaluationSeriesData(), res.data[0])
			assert.Equal(t,
				ldhooks.NewEvaluationSeriesBuilder(
					ldhooks.EmptyEvaluationSeriesData()).
					Set("testB", "testB").
					Build(), res.data[1])
			assert.Equal(t, ldvalue.Bool(false), res.context.DefaultValue())

			assert.Equal(t, []string{"During evaluation of flag \"test-flag\", an error was encountered in \"BeforeEvaluation\" of the \"a\" hook: something bad"},
				mockLog.GetOutput(ldlog.Error))
		})

		t.Run("run after evaluation with an error", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			hookA := sharedtest.NewTestHook("a")
			// The hooks execute in reverse order, so we have an error in B and check that A still executes.
			hookA.AfterInject = func(
				ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
				detail ldreason.EvaluationDetail,
			) (ldhooks.EvaluationSeriesData, error) {
				return ldhooks.NewEvaluationSeriesBuilder(data).
					Set("testA", "testA").
					Build(), nil
			}
			hookB := sharedtest.NewTestHook("b")
			hookB.AfterInject = func(
				ctx context.Context,
				seriesContext ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
				detail ldreason.EvaluationDetail,
			) (ldhooks.EvaluationSeriesData, error) {
				return ldhooks.NewEvaluationSeriesBuilder(data).
					Set("testB", "B").
					Build(), errors.New("something bad")

			}

			runner := newHookRunner(mockLog.Loggers, []ldhooks.Hook{hookA, hookB})
			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")
			detail := ldreason.NewEvaluationDetail(ldvalue.Bool(false), 0,
				ldreason.NewEvalReasonFallthrough())

			res := runner.afterEvaluation(context.Background(), execution, detail)
			assert.Len(t, res.hooks, 2)
			assert.Len(t, res.data, 2)
			assert.Equal(t, ldContext, res.context.Context())
			assert.Equal(t, "test-flag", res.context.FlagKey())
			assert.Equal(t, "testMethod", res.context.Method())
			assert.Equal(t, ldhooks.EmptyEvaluationSeriesData(), res.data[1])
			assert.Equal(t,
				ldhooks.NewEvaluationSeriesBuilder(
					ldhooks.EmptyEvaluationSeriesData()).
					Set("testA", "testA").
					Build(), res.data[0])
			assert.Equal(t, ldvalue.Bool(false), res.context.DefaultValue())
			assert.Equal(t, []string{"During evaluation of flag \"test-flag\", an error was encountered in \"AfterEvaluation\" of the \"b\" hook: something bad"},
				mockLog.GetOutput(ldlog.Error))
		})
	})
}

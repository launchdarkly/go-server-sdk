package ldclient

import (
	"context"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
)

type evalCapture struct {
	evaluationSeriesContext ldhooks.EvaluationSeriesContext
	evaluationSeriesData    ldhooks.EvaluationSeriesData
	detail                  ldreason.EvaluationDetail
}

type testHook struct {
	captureBefore []evalCapture
	captureAfter  []evalCapture
	metadata      ldhooks.HookMetadata
}

func newTestHook(name string) testHook {
	return testHook{
		captureBefore: make([]evalCapture, 0),
		captureAfter:  make([]evalCapture, 0),
		metadata:      ldhooks.NewHookMetadata(name),
	}
}

func (t testHook) GetMetadata() ldhooks.HookMetadata {
	return t.metadata
}

func (t testHook) BeforeEvaluation(_ context.Context, seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData) ldhooks.EvaluationSeriesData {
	t.captureBefore = append(t.captureBefore, evalCapture{
		evaluationSeriesContext: seriesContext,
		evaluationSeriesData:    data,
	})
	return data
}

func (t testHook) AfterEvaluation(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData, detail ldreason.EvaluationDetail) ldhooks.EvaluationSeriesData {
	t.captureAfter = append(t.captureBefore, evalCapture{
		evaluationSeriesContext: seriesContext,
		evaluationSeriesData:    data,
		detail:                  detail,
	})
	return data
}

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
		runner := newHookRunner([]ldhooks.Hook{})

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

	t.Run("create with hooks", func(t *testing.T) {
		runner := newHookRunner([]ldhooks.Hook{newTestHook("a"), newTestHook("b")})

		t.Run("prepare evaluation series", func(t *testing.T) {
			ldContext := ldcontext.New("test-context")
			res := runner.prepareEvaluationSeries("test-flag", ldContext, false, "testMethod")

			assert.Len(t, res.hooks, 2)
			assert.Len(t, res.data, 2)
			assert.Equal(t, ldContext, res.context.Context())
			assert.Equal(t, "test-flag", res.context.FlagKey())
			assert.Equal(t, "testMethod", res.context.Method())
			assert.Equal(t, ldvalue.Bool(false), res.context.DefaultValue())
		})

		t.Run("run before evaluation", func(t *testing.T) {
			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")
			_ = runner.beforeEvaluation(context.Background(), execution)

			// TODO: Implement assertions.
		})

		t.Run("run after evaluation", func(t *testing.T) {
			ldContext := ldcontext.New("test-context")
			execution := runner.prepareEvaluationSeries("test-flag", ldContext, false,
				"testMethod")
			_ = runner.afterEvaluation(context.Background(), execution,
				ldreason.NewEvaluationDetail(ldvalue.Bool(false), 0,
					ldreason.NewEvalReasonFallthrough()))

			// TODO: Implement assertions.
		})
	})
}

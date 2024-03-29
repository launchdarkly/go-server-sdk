package hooks

import (
	gocontext "context"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// Runner manages the registration and execution of hooks.
type Runner struct {
	hooks   []ldhooks.Hook
	loggers ldlog.Loggers
}

// NewRunner creates a new hook runner.
func NewRunner(loggers ldlog.Loggers, hooks []ldhooks.Hook) *Runner {
	return &Runner{
		loggers: loggers,
		hooks:   hooks,
	}
}

// RunEvaluation runs the evaluation series surrounding the given evaluation function.
func (h *Runner) RunEvaluation(
	ctx gocontext.Context,
	flagKey string,
	evalContext ldcontext.Context,
	defaultVal ldvalue.Value,
	method string,
	fn func() (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error),
) (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
	if len(h.hooks) == 0 {
		return fn()
	}
	e := h.prepareEvaluationSeries(flagKey, evalContext, defaultVal, method)
	e.BeforeEvaluation(ctx)
	detail, flag, err := fn()
	e.AfterEvaluation(ctx, detail)
	return detail, flag, err
}

// PrepareEvaluationSeries creates an EvaluationExecution suitable for executing evaluation stages.
func (h *Runner) prepareEvaluationSeries(
	flagKey string,
	evalContext ldcontext.Context,
	defaultVal ldvalue.Value,
	method string,
) *EvaluationExecution {
	returnData := make([]ldhooks.EvaluationSeriesData, len(h.hooks))
	for i := range h.hooks {
		returnData[i] = ldhooks.EmptyEvaluationSeriesData()
	}
	return &EvaluationExecution{
		hooks:   h.hooks,
		data:    returnData,
		context: ldhooks.NewEvaluationSeriesContext(flagKey, evalContext, defaultVal, method),
		loggers: &h.loggers,
	}
}

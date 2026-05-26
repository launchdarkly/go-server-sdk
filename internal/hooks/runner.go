package hooks

import (
	gocontext "context"

	"github.com/launchdarkly/go-sdk-common/v4/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v4/ldlog"
	"github.com/launchdarkly/go-sdk-common/v4/ldreason"
	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v4/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// Runner manages the registration and execution of hooks.
type Runner struct {
	hooks                 []ldhooks.Hook
	loggers               ldlog.Loggers
	environmentIDProvider internal.EnvironmentIDProvider
}

// NewRunner creates a new hook runner.
func NewRunner(
	loggers ldlog.Loggers,
	hooks []ldhooks.Hook,
	environmentIDProvider internal.EnvironmentIDProvider,
) *Runner {
	return &Runner{
		loggers:               loggers,
		hooks:                 hooks,
		environmentIDProvider: environmentIDProvider,
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
		hooks: h.hooks,
		data:  returnData,
		context: ldhooks.NewEvaluationSeriesContext(
			flagKey,
			evalContext,
			defaultVal,
			method,
			h.environmentIDProvider.GetEnvironmentID(),
		),
		loggers: &h.loggers,
	}
}

// RunTrack runs the track series after the given track function.
func (h *Runner) RunTrack(
	ctx gocontext.Context,
	key string,
	trackContext ldcontext.Context,
	metricValue *float64,
	data ldvalue.Value,
	fn func(),
) {
	fn()
	if len(h.hooks) == 0 {
		return
	}
	e := TrackExecution{
		hooks: h.hooks,
		context: ldhooks.NewTrackSeriesContext(
			trackContext,
			key,
			metricValue,
			data,
			h.environmentIDProvider.GetEnvironmentID(),
		),
		loggers: &h.loggers,
	}
	e.AfterTrack(ctx)
}

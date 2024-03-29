package hooks

import (
	gocontext "context"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// EvaluationExecution represents the state of a running series of evaluation stages.
type EvaluationExecution struct {
	hooks   []ldhooks.Hook
	data    []ldhooks.EvaluationSeriesData
	context ldhooks.EvaluationSeriesContext
	loggers *ldlog.Loggers
}

// BeforeEvaluation executes the BeforeEvaluation stage of registered hooks.
func (e *EvaluationExecution) BeforeEvaluation(ctx gocontext.Context) {
	e.executeStage(
		false,
		"BeforeEvaluation",
		func(hook ldhooks.Hook, data ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error) {
			return hook.BeforeEvaluation(ctx, e.context, data)
		})
}

// AfterEvaluation executes the AfterEvaluation stage of registered hooks.
func (e *EvaluationExecution) AfterEvaluation(
	ctx gocontext.Context,
	detail ldreason.EvaluationDetail,
) {
	e.executeStage(
		true,
		"AfterEvaluation",
		func(hook ldhooks.Hook, data ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error) {
			return hook.AfterEvaluation(ctx, e.context, data, detail)
		})
}

func (e *EvaluationExecution) executeStage(
	reverse bool,
	stageName string,
	fn func(
		hook ldhooks.Hook,
		data ldhooks.EvaluationSeriesData,
	) (ldhooks.EvaluationSeriesData, error)) {
	returnData := make([]ldhooks.EvaluationSeriesData, len(e.hooks))
	iterator := newIterator(reverse, e.hooks)
	for iterator.hasNext() {
		i, hook := iterator.getNext()

		outData, err := fn(hook, e.data[i])
		if err != nil {
			returnData[i] = e.data[i]
			e.loggers.Errorf(
				"During evaluation of flag \"%s\", an error was encountered in \"%s\" of the \"%s\" hook: %s",
				e.context.FlagKey(),
				stageName,
				hook.Metadata().Name(),
				err.Error())
			continue
		}
		returnData[i] = outData
	}
	e.data = returnData
}

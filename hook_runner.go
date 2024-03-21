package ldclient

import (
	"context"
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

type hookRunner struct {
	hooks []ldhooks.Hook
	mutex *sync.RWMutex
}

type evaluationExecution struct {
	hooks   []ldhooks.Hook
	data    []ldhooks.EvaluationSeriesData
	context ldhooks.EvaluationSeriesContext
}

func (e evaluationExecution) withData(data []ldhooks.EvaluationSeriesData) evaluationExecution {
	return evaluationExecution{
		hooks:   e.hooks,
		context: e.context,
		data:    data,
	}
}

func newHookRunner(hooks []ldhooks.Hook) hookRunner {
	return hookRunner{
		hooks: hooks,
		mutex: &sync.RWMutex{},
	}
}

func (h hookRunner) addHook(hook ldhooks.Hook) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.hooks = append(h.hooks, hook)
}

// getHooks returns a copy of the hooks. This copy is suitable for use when executing a series. This keeps the set
// of hooks stable for the duration of the series. This prevents things like calling the afterEvaluation method for
// a hook that didn't have the beforeEvaluation method called.
func (h hookRunner) getHooks() []ldhooks.Hook {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	copiedHooks := make([]ldhooks.Hook, len(h.hooks))
	copy(copiedHooks, h.hooks)
	return copiedHooks
}

func (h hookRunner) prepareEvaluationSeries(flagKey string, evalContext ldcontext.Context, defaultVal any, method string) evaluationExecution {
	hooksForEval := h.getHooks()
	returnData := make([]ldhooks.EvaluationSeriesData, len(hooksForEval))
	for i := range hooksForEval {
		returnData[i] = ldhooks.EmptyEvaluationSeriesData()
	}
	return evaluationExecution{
		hooks:   hooksForEval,
		data:    returnData,
		context: ldhooks.NewEvaluationSeriesContext(flagKey, evalContext, defaultVal, method),
	}
}

func (h hookRunner) beforeEvaluation(ctx context.Context, execution evaluationExecution) evaluationExecution {
	returnData := make([]ldhooks.EvaluationSeriesData, len(execution.hooks))

	for i, hook := range execution.hooks {
		outData := hook.BeforeEvaluation(ctx, execution.context, execution.data[i])
		returnData[i] = outData
	}

	return execution.withData(returnData)
}

func (h hookRunner) afterEvaluation(ctx context.Context, execution evaluationExecution, detail ldreason.EvaluationDetail) evaluationExecution {

	returnData := make([]ldhooks.EvaluationSeriesData, len(execution.hooks))
	for i := len(execution.hooks) - 1; i >= 0; i-- {
		hook := execution.hooks[i]
		outData := hook.AfterEvaluation(ctx, execution.context, execution.data[i], detail)
		returnData[i] = outData
	}
	return execution.withData(returnData)
}

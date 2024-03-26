package hooks

import (
	"context"
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// HookRunner manages the registration and execution of hooks.
type HookRunner struct {
	hooks   []ldhooks.Hook
	loggers ldlog.Loggers
	mutex   *sync.RWMutex
}

// EvaluationExecution represents the state of a running series of evaluation stages.
type EvaluationExecution struct {
	hooks   []ldhooks.Hook
	data    []ldhooks.EvaluationSeriesData
	context ldhooks.EvaluationSeriesContext
}

func (e EvaluationExecution) withData(data []ldhooks.EvaluationSeriesData) EvaluationExecution {
	return EvaluationExecution{
		hooks:   e.hooks,
		context: e.context,
		data:    data,
	}
}

// NewHookRunner creates a new hook runner.
func NewHookRunner(loggers ldlog.Loggers, hooks []ldhooks.Hook) *HookRunner {
	return &HookRunner{
		loggers: loggers,
		hooks:   hooks,
		mutex:   &sync.RWMutex{},
	}
}

// AddHooks adds hooks to the runner.
func (h *HookRunner) AddHooks(hooks ...ldhooks.Hook) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.hooks = append(h.hooks, hooks...)
}

// getHooks returns a copy of the hooks. This copy is suitable for use when executing a series. This keeps the set
// of hooks stable for the duration of the series. This prevents things like calling the AfterEvaluation method for
// a hook that didn't have the BeforeEvaluation method called.
func (h *HookRunner) getHooks() []ldhooks.Hook {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	copiedHooks := make([]ldhooks.Hook, len(h.hooks))
	copy(copiedHooks, h.hooks)
	return copiedHooks
}

// PrepareEvaluationSeries creates an EvaluationExecution suitable for executing evaluation stages and gets a copy
// of hooks to use during series execution.
//
// For an invocation of a series the same set of hooks should be used. For instance a hook added mid-evaluation should
// not be executed during the "AfterEvaluation" stage of that evaluation.
func (h *HookRunner) PrepareEvaluationSeries(
	flagKey string,
	evalContext ldcontext.Context,
	defaultVal any,
	method string,
) EvaluationExecution {
	hooksForEval := h.getHooks()
	returnData := make([]ldhooks.EvaluationSeriesData, len(hooksForEval))
	for i := range hooksForEval {
		returnData[i] = ldhooks.EmptyEvaluationSeriesData()
	}
	return EvaluationExecution{
		hooks:   hooksForEval,
		data:    returnData,
		context: ldhooks.NewEvaluationSeriesContext(flagKey, evalContext, defaultVal, method),
	}
}

// BeforeEvaluation executes the BeforeEvaluation stage of registered hooks.
func (h *HookRunner) BeforeEvaluation(ctx context.Context, execution EvaluationExecution) EvaluationExecution {
	return h.executeStage(
		execution,
		false,
		"BeforeEvaluation",
		func(hook ldhooks.Hook, data ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error) {
			return hook.BeforeEvaluation(ctx, execution.context, data)
		})
}

// AfterEvaluation executes the AfterEvaluation stage of registered hooks.
func (h *HookRunner) AfterEvaluation(
	ctx context.Context,
	execution EvaluationExecution,
	detail ldreason.EvaluationDetail,
) EvaluationExecution {
	return h.executeStage(
		execution,
		true,
		"AfterEvaluation",
		func(hook ldhooks.Hook, data ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error) {
			return hook.AfterEvaluation(ctx, execution.context, data, detail)
		})
}

func (h *HookRunner) executeStage(
	execution EvaluationExecution,
	reverse bool,
	stageName string,
	fn func(
		hook ldhooks.Hook,
		data ldhooks.EvaluationSeriesData,
	) (ldhooks.EvaluationSeriesData, error)) EvaluationExecution {
	returnData := make([]ldhooks.EvaluationSeriesData, len(execution.hooks))
	iterator := newHookIterator(reverse, &execution.hooks)
	for iterator.hasNext() {
		i, hook := iterator.getNext()

		outData, err := fn(hook, execution.data[i])
		if err != nil {
			returnData[i] = execution.data[i]
			h.loggers.Errorf(
				"During evaluation of flag \"%s\", an error was encountered in \"%s\" of the \"%s\" hook: %s",
				execution.context.FlagKey(),
				stageName,
				hook.GetMetadata().Name(),
				err.Error())
			continue
		}
		returnData[i] = outData
	}
	return execution.withData(returnData)
}

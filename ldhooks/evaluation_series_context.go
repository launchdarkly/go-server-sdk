package ldhooks

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// EvaluationSeriesContext contains contextual information for the execution of stages in the evaluation series.
type EvaluationSeriesContext struct {
	flagKey      string
	context      ldcontext.Context
	defaultValue ldvalue.Value
	method       string
}

// NewEvaluationSeriesContext create a new EvaluationSeriesContext. Hook implementations do not need to use this function.
func NewEvaluationSeriesContext(flagKey string, evalContext ldcontext.Context,
	defaultValue any, method string) EvaluationSeriesContext {
	return EvaluationSeriesContext{
		flagKey:      flagKey,
		context:      evalContext,
		defaultValue: ldvalue.CopyArbitraryValue(defaultValue),
		method:       method,
	}
}

func (c EvaluationSeriesContext) FlagKey() string {
	return c.flagKey
}

func (c EvaluationSeriesContext) Context() ldcontext.Context {
	return c.context
}

func (c EvaluationSeriesContext) DefaultValue() ldvalue.Value {
	return c.defaultValue
}

func (c EvaluationSeriesContext) Method() string {
	return c.method
}

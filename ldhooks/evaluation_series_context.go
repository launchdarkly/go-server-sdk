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
func NewEvaluationSeriesContext(flagKey string, context ldcontext.Context,
	defaultValue any, method string) EvaluationSeriesContext {
	return EvaluationSeriesContext{
		flagKey:      flagKey,
		context:      context,
		defaultValue: ldvalue.CopyArbitraryValue(defaultValue),
		method:       method,
	}
}

func (c EvaluationSeriesContext) GetFlagKey() string {
	return c.flagKey
}

func (c EvaluationSeriesContext) GetContext() ldcontext.Context {
	return c.context
}

func (c EvaluationSeriesContext) GetDefaultValue() ldvalue.Value {
	return c.defaultValue
}

func (c EvaluationSeriesContext) GetMethod() string {
	return c.method
}

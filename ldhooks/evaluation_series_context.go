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

// FlagKey gets the key of the flag being evaluated.
func (c EvaluationSeriesContext) FlagKey() string {
	return c.flagKey
}

// Context gets the evaluation context the flag is being evaluated for.
func (c EvaluationSeriesContext) Context() ldcontext.Context {
	return c.context
}

// DefaultValue gets the default value for the evaluation.
func (c EvaluationSeriesContext) DefaultValue() ldvalue.Value {
	return c.defaultValue
}

// Method gets a string represent of the LDClient method being executed.
func (c EvaluationSeriesContext) Method() string {
	return c.method
}

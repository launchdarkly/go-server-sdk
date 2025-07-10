package ldhooks

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// EvaluationSeriesContext contains contextual information for the execution of stages in the evaluation series.
type EvaluationSeriesContext struct {
	flagKey       string
	context       ldcontext.Context
	defaultValue  ldvalue.Value
	method        string
	environmentID ldvalue.OptionalString
}

// NewEvaluationSeriesContext create a new EvaluationSeriesContext. Hook implementations do not need to use this
// function.
func NewEvaluationSeriesContext(
	flagKey string,
	evalContext ldcontext.Context,
	defaultValue ldvalue.Value,
	method string,
	environmentID ldvalue.OptionalString,
) EvaluationSeriesContext {
	return EvaluationSeriesContext{
		flagKey:       flagKey,
		context:       evalContext,
		defaultValue:  defaultValue,
		method:        method,
		environmentID: environmentID,
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

// EnvironmentID gets the ID of the environment where the flag is being evaluated.
func (c EvaluationSeriesContext) EnvironmentID() ldvalue.OptionalString {
	return c.environmentID
}

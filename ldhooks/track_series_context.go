package ldhooks

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// TrackSeriesContext contains contextual information for the execution of stages in the evaluation series.
type TrackSeriesContext struct {
	context       ldcontext.Context
	key           string
	metricValue   *float64
	data          ldvalue.Value
	environmentID ldvalue.OptionalString
}

// NewTrackSeriesContext create a new TrackSeriesContext. Hook implementations do not need to use this
// function.
func NewTrackSeriesContext(
	context ldcontext.Context,
	key string,
	metricValue *float64,
	data ldvalue.Value,
	environmentID ldvalue.OptionalString,
) TrackSeriesContext {
	return TrackSeriesContext{
		context:       context,
		key:           key,
		metricValue:   metricValue,
		data:          data,
		environmentID: environmentID,
	}
}

// Context gets the evaluation context the event is being tracked for.
func (c TrackSeriesContext) Context() ldcontext.Context {
	return c.context
}

// Key gets the key associated with the track call.
func (c TrackSeriesContext) Key() string {
	return c.key
}

// MetricValue gets the metric value associated with the track call.
func (c TrackSeriesContext) MetricValue() *float64 {
	return c.metricValue
}

// Data gets any application-specified data associated with track call.
func (c TrackSeriesContext) Data() ldvalue.Value {
	return c.data
}

// EnvironmentID gets the ID of the environment where the event is being tracked.
func (c TrackSeriesContext) EnvironmentID() ldvalue.OptionalString {
	return c.environmentID
}

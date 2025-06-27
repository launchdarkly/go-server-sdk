package ldhooks

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// TrackSeriesContext contains contextual information for the execution of stages in the evaluation series.
type TrackSeriesContext struct {
	context     ldcontext.Context
	key         string
	hasMetric   bool
	metricValue *float64
	data        ldvalue.Value
}

// NewTrackSeriesContext create a new TrackSeriesContext. Hook implementations do not need to use this
// function.
func NewTrackSeriesContext(
	context ldcontext.Context,
	key string,
	metricValue *float64,
	data ldvalue.Value,
) TrackSeriesContext {
	return TrackSeriesContext{
		context:     context,
		key:         key,
		metricValue: metricValue,
		data:        data,
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

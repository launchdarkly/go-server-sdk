package ldevents

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
)

var defaultEventFactory = NewEventFactory(false, nil)

var noReason = ldreason.EvaluationReason{}

// Stub implementation of FlagEventProperties
type flagEventPropertiesImpl struct {
	Key                  string
	Version              int
	TrackEvents          bool
	DebugEventsUntilDate ldtime.UnixMillisecondTime
	IsExperiment         bool
}

func (f flagEventPropertiesImpl) GetKey() string                   { return f.Key }
func (f flagEventPropertiesImpl) GetVersion() int                  { return f.Version }
func (f flagEventPropertiesImpl) IsFullEventTrackingEnabled() bool { return f.TrackEvents }
func (f flagEventPropertiesImpl) GetDebugEventsUntilDate() ldtime.UnixMillisecondTime {
	return f.DebugEventsUntilDate
}
func (f flagEventPropertiesImpl) IsExperimentationEnabled(reason ldreason.EvaluationReason) bool {
	return f.IsExperiment
}

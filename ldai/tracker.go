package ldai

import (
	"fmt"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"time"
)

const (
	duration         = "$ld:ai:duration:total"
	feedbackPositive = "$ld:ai:feedback:user:positive"
	feedbackNegative = "$ld:ai:feedback:user:negative"
	generation       = "$ld:ai:generation"
	tokenTotal       = "$ld:ai:tokens:total"
	tokenInput       = "$ld:ai:tokens:input"
	tokenOutput      = "$ld:ai:tokens:output"
)

type TokenUsage struct {
	Total  int
	Input  int
	Output int
}

func (t TokenUsage) Set() bool {
	return t.Total > 0 || t.Input > 0 || t.Output > 0
}

type Metrics struct {
	LatencyMs float64
}

func (m Metrics) Set() bool {
	return m.LatencyMs != 0
}

type ProviderResponse struct {
	Usage   TokenUsage
	Metrics Metrics
}

type Feedback string

const (
	Positive Feedback = "positive"
	Negative Feedback = "negative"
)

type Tracker struct {
	key       string
	config    *Config
	context   ldcontext.Context
	events    EventTracker
	trackData ldvalue.Value
	logger    interfaces.LDLoggers
}

type EventTracker interface {
	TrackMetric(
		eventName string,
		context ldcontext.Context,
		metricValue float64,
		data ldvalue.Value,
	) error
}

func NewTracker(key string, events EventTracker, config *Config, ctx ldcontext.Context, loggers interfaces.LDLoggers) *Tracker {
	if config == nil {
		panic("LaunchDarkly SDK programmer error: config must never be nil")
	}

	trackData := ldvalue.ObjectBuild().
		Set("versionKey", ldvalue.String(config.VersionKey())).
		Set("configKey", ldvalue.String(key)).Build()

	return &Tracker{
		key:       key,
		config:    config,
		trackData: trackData,
		events:    events,
		context:   ctx,
		logger:    loggers,
	}
}

func (t *Tracker) logWarning(format string, args ...interface{}) {
	prefix := "AI config tracker for '" + t.key + "': "
	t.logger.Warnf(prefix+format, args...)
}

func (t *Tracker) TrackDuration(durationMs float64) error {
	return t.events.TrackMetric(duration, t.context, durationMs, t.trackData)
}

func (t *Tracker) TrackFeedback(feedback Feedback) error {
	switch feedback {
	case Positive:
		return t.events.TrackMetric(feedbackPositive, t.context, 1, t.trackData)
	case Negative:
		return t.events.TrackMetric(feedbackNegative, t.context, 1, t.trackData)
	default:
		return fmt.Errorf("tracker: unexpected feedback value: %v", feedback)
	}
}

func (t *Tracker) TrackSuccess() error {
	return t.events.TrackMetric(generation, t.context, 1, t.trackData)
}

func (t *Tracker) TrackUsage(usage TokenUsage) error {
	var failed bool

	if usage.Total > 0 {
		if err1 := t.events.TrackMetric(tokenTotal, t.context, float64(usage.Total), t.trackData); err1 != nil {
			t.logWarning("error tracking total token usage: %v", err1)
			failed = true
		}
	}
	if usage.Input > 0 {
		if err2 := t.events.TrackMetric(tokenInput, t.context, float64(usage.Input), t.trackData); err2 != nil {
			t.logWarning("error tracking input token usage: %v", err2)
			failed = true
		}
	}
	if usage.Output > 0 {
		if err3 := t.events.TrackMetric(tokenOutput, t.context, float64(usage.Output), t.trackData); err3 != nil {
			t.logWarning("error tracking output token usage: %v", err3)
			failed = true
		}
	}

	if failed {
		return fmt.Errorf("tracker: error tracking token usage, logs contain more information")
	}

	return nil
}

func measureDurationOfTask[T any](task func() (T, error)) (T, int64, error) {
	start := time.Now()
	result, err := task()
	duration := time.Since(start).Milliseconds()
	return result, duration, err
}

func (t *Tracker) TrackRequest(task func() (ProviderResponse, error)) (ProviderResponse, error) {
	usage, duration, err := measureDurationOfTask(task)

	if err != nil {
		t.logWarning("error executing request: %v", err)
		return ProviderResponse{}, err
	}
	if err := t.TrackSuccess(); err != nil {
		t.logWarning("error tracking success metric for request: %v", err)
	}

	if usage.Metrics.Set() {
		if err := t.TrackDuration(usage.Metrics.LatencyMs); err != nil {
			t.logWarning("error tracking duration metric (user provided) for request: %v", err)
		}
	} else {
		if err := t.TrackDuration(float64(duration)); err != nil {
			t.logWarning("error tracking duration metric (automatically measured) for request: %v", err)
		}
	}

	if usage.Usage.Set() {
		// TrackUsage logs errors.
		_ = t.TrackUsage(usage.Usage)
	}

	return usage, nil
}

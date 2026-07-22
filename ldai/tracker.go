package ldai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	ldcommon "github.com/launchdarkly/go-sdk-common/v3"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

const (
	duration          = "$ld:ai:duration:total"
	feedbackPositive  = "$ld:ai:feedback:user:positive"
	feedbackNegative  = "$ld:ai:feedback:user:negative"
	generationSuccess = "$ld:ai:generation:success"
	generationError   = "$ld:ai:generation:error"
	//nolint:gosec
	timeToFirstToken = "$ld:ai:tokens:ttf"
	//nolint:gosec
	tokenTotal = "$ld:ai:tokens:total"
	//nolint:gosec
	tokenInput = "$ld:ai:tokens:input"
	//nolint:gosec
	tokenOutput = "$ld:ai:tokens:output"
)

// newRunID returns a fresh UUIDv4 that LaunchDarkly uses to group all metric
// events emitted by one Tracker into a single AI run, so they can be analyzed
// together. Each call to CreateTracker on the AI Config mints a new runId.
func newRunID() string {
	return uuid.New().String()
}

// TokenUsage represents the token usage returned by a model provider for a specific request.
type TokenUsage struct {
	// Total is the total number of tokens used.
	Total int
	// Input is the number of input tokens used.
	Input int

	// Output is the number of output tokens used.
	Output int
}

// MetricSummary represents a summary of metrics tracked by the tracker.
type MetricSummary struct {
	// Duration is the tracked duration in milliseconds.
	Duration ldcommon.Option[time.Duration]
	// Feedback is the tracked user feedback (positive or negative).
	Feedback ldcommon.Option[Feedback]
	// Tokens contains information about token usage.
	Tokens ldcommon.Option[TokenUsage]
	// Success indicates whether the operation was successful.
	Success ldcommon.Option[bool]
	// TimeToFirstToken is the time to the first token in milliseconds.
	TimeToFirstToken ldcommon.Option[time.Duration]
}

// Set returns true if any of the fields are non-zero.
func (t TokenUsage) Set() bool {
	return t.Total > 0 || t.Input > 0 || t.Output > 0
}

// Metrics represents the metrics returned by a model provider for a specific request.
type Metrics struct {
	// Latency is the latency of the request.
	Latency time.Duration
	// TimeToFirstToken is the time to the first token of the streamed response.
	TimeToFirstToken time.Duration
}

// ProviderResponse represents the response from a model provider for a specific request.
type ProviderResponse struct {
	// Usage is the token usage.
	Usage TokenUsage
	// Metrics is the request metrics.
	Metrics Metrics
}

// Feedback represents the feedback provided by a user for a model evaluation.
type Feedback string

const (
	// FeedbackPositive is positive feedback.
	FeedbackPositive Feedback = "positive"
	// FeedbackNegative is negative feedback.
	FeedbackNegative Feedback = "negative"
)

// EventSink represents the Tracker's requirements for delivering analytic events. This is generally satisfied
// by the LaunchDarkly SDK's TrackMetric method.
type EventSink interface {
	// TrackMetric sends a named analytic event to LaunchDarkly relevant to a particular context, and containing a
	// metric value and additional data.
	TrackMetric(
		eventName string,
		context ldcontext.Context,
		metricValue float64,
		data ldvalue.Value,
	) error
}

// Stopwatch is used to measure the duration of a task. Start will always be called before Stop.
// If an implementation is not provided, the Tracker uses a default implementation that delegates to
// time.Now and time.Since.
type Stopwatch interface {
	// Start starts the stopwatch.
	Start()
	// Stop stops the stopwatch and returns the duration since Start was called.
	Stop() time.Duration
}

// resumptionPayload is the JSON structure encoded into a resumption token.
type resumptionPayload struct {
	RunID        string `json:"runId"`
	ConfigKey    string `json:"configKey"`
	VariationKey string `json:"variationKey,omitempty"`
	Version      int    `json:"version"`
}

// Tracker records metrics for a single AI run.
// Unless otherwise noted, the Tracker's methods are not safe for concurrent use.
//
// All events a Tracker emits share a runId (a UUIDv4) so LaunchDarkly can
// correlate them in metrics views. See individual track methods for their
// specific semantics. Call CreateTracker on the AI Config to start a new run.
// A ResumptionToken preserves the runId, so events emitted by a Tracker
// reconstructed in another process correlate with the original run.
type Tracker struct {
	key          string
	runID        string
	variationKey string
	version      int
	config       *Config
	context      ldcontext.Context
	events       EventSink
	trackData    ldvalue.Value
	logger       interfaces.LDLoggers
	stopwatch    Stopwatch

	duration         ldcommon.Option[time.Duration]
	feedback         ldcommon.Option[Feedback]
	tokens           ldcommon.Option[TokenUsage]
	success          ldcommon.Option[bool]
	timeToFirstToken ldcommon.Option[time.Duration]
}

// Used if a custom Stopwatch is not provided.
type defaultStopwatch struct {
	start time.Time
}

// Start saves the current time using time.Now.
func (d *defaultStopwatch) Start() {
	d.start = time.Now()
}

// Stop returns the duration since Start was called using time.Since.
func (d *defaultStopwatch) Stop() time.Duration {
	return time.Since(d.start)
}

// newTracker creates a new Tracker with the specified runID, key, event sink, config, context, and loggers.
func newTracker(
	events EventSink,
	runID string,
	key string,
	variationKey string,
	version int,
	modelKey string,
	modelVersion int,
	ctx ldcontext.Context,
	config *Config,
	loggers interfaces.LDLoggers,
) *Tracker {
	return newTrackerWithStopwatch(
		events, runID, key, variationKey, version, modelKey, modelVersion, ctx, config, loggers, &defaultStopwatch{})
}

// newTrackerWithStopwatch creates a new Tracker with the specified runID, key, event sink, config, context, loggers,
// and stopwatch. This method is used for testing purposes.
func newTrackerWithStopwatch(
	events EventSink,
	runID string,
	key string,
	variationKey string,
	version int,
	modelKey string,
	modelVersion int,
	ctx ldcontext.Context,
	config *Config,
	loggers interfaces.LDLoggers,
	stopwatch Stopwatch,
) *Tracker {
	if config == nil {
		panic("LaunchDarkly SDK programmer error: config must never be nil")
	}

	builder := ldvalue.ObjectBuild().
		Set("runId", ldvalue.String(runID)).
		Set("configKey", ldvalue.String(key)).
		Set("version", ldvalue.Int(version)).
		Set("providerName", ldvalue.String(config.ProviderName())).
		Set("modelName", ldvalue.String(config.ModelName())).
		Set("modelVersion", ldvalue.Int(modelVersion))
	if modelKey != "" {
		builder.Set("modelKey", ldvalue.String(modelKey))
	}
	if variationKey != "" {
		builder.Set("variationKey", ldvalue.String(variationKey))
	}
	trackData := builder.Build()

	return &Tracker{
		key:          key,
		runID:        runID,
		variationKey: variationKey,
		version:      version,
		config:       config,
		trackData:    trackData,
		events:       events,
		context:      ctx,
		logger:       loggers,
		stopwatch:    stopwatch,
	}
}

func (t *Tracker) logWarning(format string, args ...interface{}) {
	prefix := "AI Config tracker for '" + t.key + "': "
	t.logger.Warnf(prefix+format, args...)
}

// ResumptionToken returns a URL-safe Base64-encoded token that can be used to reconstruct a tracker
// in a different process (e.g., for deferred feedback). The token contains the runId, configKey,
// variationKey, and version. It does not contain modelName, providerName, modelKey, or modelVersion.
func (t *Tracker) ResumptionToken() string {
	payload := resumptionPayload{
		RunID:        t.runID,
		ConfigKey:    t.key,
		VariationKey: t.variationKey,
		Version:      t.version,
	}
	jsonBytes, _ := json.Marshal(payload)
	return base64.RawURLEncoding.EncodeToString(jsonBytes)
}

// TrackerFromResumptionToken reconstructs a Tracker from a resumption token and the given context.
// This is used for cross-process scenarios (e.g., deferred feedback) where the original tracker
// is no longer available but its runId must be reused. The token is obtained from Tracker.ResumptionToken().
// The reconstructed tracker will have empty modelName, providerName, and modelKey, and modelVersion
// defaults to 1, since these are not included in the token.
func TrackerFromResumptionToken(token string, sdk ServerSDK, context ldcontext.Context) (*Tracker, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid resumption token: %w", err)
	}
	var payload resumptionPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("invalid resumption token: %w", err)
	}

	builder := ldvalue.ObjectBuild().
		Set("runId", ldvalue.String(payload.RunID)).
		Set("configKey", ldvalue.String(payload.ConfigKey)).
		Set("version", ldvalue.Int(payload.Version)).
		Set("providerName", ldvalue.String("")).
		Set("modelName", ldvalue.String("")).
		Set("modelVersion", ldvalue.Int(1))
	if payload.VariationKey != "" {
		builder.Set("variationKey", ldvalue.String(payload.VariationKey))
	}
	trackData := builder.Build()

	emptyConfig := Disabled()

	return &Tracker{
		key:          payload.ConfigKey,
		runID:        payload.RunID,
		variationKey: payload.VariationKey,
		version:      payload.Version,
		config:       &emptyConfig,
		trackData:    trackData,
		events:       sdk,
		context:      context,
		logger:       sdk.Loggers(),
		stopwatch:    &defaultStopwatch{},
	}, nil
}

// TrackDuration tracks the duration of a task. For example, the duration of a model evaluation request may be
// tracked here. See also TrackRequest.
// The duration in milliseconds must fit within a float64.
//
// Records at most once per Tracker; further calls are ignored.
func (t *Tracker) TrackDuration(dur time.Duration) error {
	if t.duration.IsSome() {
		t.logWarning("Skipping TrackDuration: duration already recorded on this tracker. "+
			"Call CreateTracker on the AI Config for a new run. %s", t.trackData.JSONString())
		return nil
	}
	t.duration = ldcommon.Some(dur)
	return t.events.TrackMetric(duration, t.context, float64(dur.Milliseconds()), t.trackData)
}

// TrackFeedback tracks the feedback provided by a user for a model evaluation. If the feedback is not
// FeedbackPositive or FeedbackNegative, returns an error and does not track anything.
//
// Records at most once per Tracker; further calls are ignored.
func (t *Tracker) TrackFeedback(feedback Feedback) error {
	if t.feedback.IsSome() {
		t.logWarning("Skipping TrackFeedback: feedback already recorded on this tracker. "+
			"Call CreateTracker on the AI Config for a new run. %s", t.trackData.JSONString())
		return nil
	}
	switch feedback {
	case FeedbackPositive:
		t.feedback = ldcommon.Some(feedback)
		return t.events.TrackMetric(feedbackPositive, t.context, 1, t.trackData)
	case FeedbackNegative:
		t.feedback = ldcommon.Some(feedback)
		return t.events.TrackMetric(feedbackNegative, t.context, 1, t.trackData)
	default:
		return fmt.Errorf("tracker: unexpected feedback value: %v", feedback)
	}
}

// TrackSuccess tracks a successful model evaluation.
//
// Records at most once per Tracker. TrackSuccess and TrackError share state;
// only one of the two can record per Tracker, and subsequent calls are ignored.
func (t *Tracker) TrackSuccess() error {
	if t.success.IsSome() {
		t.logWarning("Skipping TrackSuccess: success/error already recorded on this tracker. "+
			"Call CreateTracker on the AI Config for a new run. %s", t.trackData.JSONString())
		return nil
	}
	t.success = ldcommon.Some(true)

	return t.events.TrackMetric(generationSuccess, t.context, 1, t.trackData)
}

// TrackError tracks an unsuccessful model evaluation.
//
// Records at most once per Tracker. TrackSuccess and TrackError share state;
// only one of the two can record per Tracker, and subsequent calls are ignored.
func (t *Tracker) TrackError() error {
	if t.success.IsSome() {
		t.logWarning("Skipping TrackError: success/error already recorded on this tracker. "+
			"Call CreateTracker on the AI Config for a new run. %s", t.trackData.JSONString())
		return nil
	}
	t.success = ldcommon.Some(false)

	return t.events.TrackMetric(generationError, t.context, 1, t.trackData)
}

// TrackTimeToFirstToken tracks the time to the first token of the streamed response.
//
// Records at most once per Tracker; further calls are ignored.
func (t *Tracker) TrackTimeToFirstToken(dur time.Duration) error {
	if t.timeToFirstToken.IsSome() {
		t.logWarning("Skipping TrackTimeToFirstToken: time-to-first-token already recorded on this tracker. "+
			"Call CreateTracker on the AI Config for a new run. %s", t.trackData.JSONString())
		return nil
	}
	t.timeToFirstToken = ldcommon.Some(dur)
	return t.events.TrackMetric(timeToFirstToken, t.context, float64(dur.Milliseconds()), t.trackData)
}

// TrackTokens tracks the token usage for a model evaluation.
//
// Records at most once per Tracker; further calls are ignored.
func (t *Tracker) TrackTokens(usage TokenUsage) error {
	if t.tokens.IsSome() {
		t.logWarning("Skipping TrackTokens: token usage already recorded on this tracker. "+
			"Call CreateTracker on the AI Config for a new run. %s", t.trackData.JSONString())
		return nil
	}

	if usage.Set() {
		t.tokens = ldcommon.Some(usage)
	}

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

// TrackUsage tracks token usage.
//
// Deprecated: Use TrackTokens instead.
func (t *Tracker) TrackUsage(usage TokenUsage) error {
	return t.TrackTokens(usage)
}

func measureDurationOfTask[T any, A any](
	stopwatch Stopwatch,
	arg A,
	task func(A) (T, error),
) (T, time.Duration, error) {
	stopwatch.Start()
	result, err := task(arg)
	return result, stopwatch.Stop(), err
}

// GetSummary returns a summary of all metrics that have been tracked using this tracker.
func (t *Tracker) GetSummary() MetricSummary {
	return MetricSummary{
		Duration:         t.duration,
		Feedback:         t.feedback,
		Tokens:           t.tokens,
		Success:          t.success,
		TimeToFirstToken: t.timeToFirstToken,
	}
}

// TrackRequest tracks metrics for a model evaluation request. The task function should return a ProviderResponse
// which can be used to specify request metrics and token usage. All fields of the returned ProviderResponse are
// optional.
//
// The task function will be passed the current AI Config, which can be used to obtain any parameters or messages
// relevant to the request.
//
// If the task returns an error, then the request is not considered successful and no metrics are tracked.
// Otherwise, the following metrics are tracked:
//  1. Successful model evaluation.
//  2. Any metrics that were that set in the ProviderResponse
//     2a) If Latency was not set in the ProviderResponse's Metrics field, an automatically measured duration.
//  3. Any token usage that was set in the ProviderResponse.
//
// Subsequent calls re-run the task but emit only metrics not already recorded
// on this Tracker. Call CreateTracker on the AI Config to start a new run.
func (t *Tracker) TrackRequest(task func(c *Config) (ProviderResponse, error)) (ProviderResponse, error) {
	usage, duration, err := measureDurationOfTask(t.stopwatch, t.config, task)
	if err != nil {
		if e := t.TrackError(); e != nil {
			t.logWarning("error tracking error metric for request: %v", e)
		}

		t.logWarning("error executing request: %v", err)
		return ProviderResponse{}, err
	}
	if err := t.TrackSuccess(); err != nil {
		t.logWarning("error tracking success metric for request: %v", err)
	}

	if usage.Metrics.Latency != 0 {
		if err := t.TrackDuration(usage.Metrics.Latency); err != nil {
			t.logWarning("error tracking duration metric (user provided) for request: %v", err)
		}
	} else {
		if err := t.TrackDuration(duration); err != nil {
			t.logWarning("error tracking duration metric (automatically measured) for request: %v", err)
		}
	}

	if usage.Metrics.TimeToFirstToken != 0 {
		if err := t.TrackTimeToFirstToken(usage.Metrics.TimeToFirstToken); err != nil {
			t.logWarning("error tracking time to first token metric for request: %v", err)
		}
	}

	if usage.Usage.Set() {
		// TrackTokens logs errors.
		_ = t.TrackTokens(usage.Usage)
	}

	return usage, nil
}

// TrackJudgeResponse tracks the evaluation scores from a judge response.
//
// May be called multiple times per Tracker; each call records the scores from
// the given response.
func (t *Tracker) TrackJudgeResponse(response datamodel.JudgeResponse) error {
	if !response.Success {
		return nil
	}

	// Build the data object once, since it's constant across all iterations
	data := t.trackData
	if response.JudgeConfigKey != "" {
		builder := ldvalue.ObjectBuild()
		for _, key := range t.trackData.Keys(nil) {
			builder.Set(key, t.trackData.GetByKey(key))
		}
		data = builder.Set("judgeConfigKey", ldvalue.String(response.JudgeConfigKey)).Build()
	}

	var failed bool
	for metricKey, evalScore := range response.Evals {
		if err := t.events.TrackMetric(metricKey, t.context, evalScore.Score, data); err != nil {
			t.logWarning("error tracking metric %s: %v", metricKey, err)
			failed = true
		}
	}

	if failed {
		return fmt.Errorf("error tracking evaluation scores")
	}
	return nil
}

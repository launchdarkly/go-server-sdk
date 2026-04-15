package ldai

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

type mockEvents struct {
	log    *ldlogtest.MockLog
	events []trackEvent
}

type trackEvent struct {
	name        string
	context     ldcontext.Context
	metricValue float64
	data        ldvalue.Value
}

func newMockEvents() *mockEvents {
	return &mockEvents{log: ldlogtest.NewMockLog()}
}

func (m *mockEvents) TrackMetric(eventName string, context ldcontext.Context, metricValue float64, data ldvalue.Value) error {
	m.events = append(m.events, trackEvent{name: eventName, context: context, metricValue: metricValue, data: data})
	return nil
}

func TestTracker_NewPanicsWithNilConfig(t *testing.T) {
	assert.Panics(t, func() {
		newTracker("key", newRunID(), "variationKey", 1, newMockEvents(), nil, ldcontext.New("key"), nil)
	})
}

func TestTracker_NewDoesNotPanicWithConfig(t *testing.T) {
	assert.NotPanics(t, func() {
		newTracker("key", newRunID(), "variationKey", 1, newMockEvents(), &Config{}, ldcontext.New("key"), nil)
	})
}

func makeTrackData(configKey, variationKey string, version int, config *Config, runId string) ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("runId", ldvalue.String(runId)).
		Set("variationKey", ldvalue.String(variationKey)).
		Set("configKey", ldvalue.String(configKey)).
		Set("version", ldvalue.Int(version)).
		Set("providerName", ldvalue.String(config.ProviderName())).
		Set("modelName", ldvalue.String(config.ModelName())).
		Build()
}

func extractRunId(t *testing.T, events *mockEvents) string {
	t.Helper()
	require.NotEmpty(t, events.events, "expected at least one event to extract runId")
	runId := events.events[0].data.GetByKey("runId").StringValue()
	require.NotEmpty(t, runId, "expected runId to be non-empty")
	return runId
}

func TestTracker_TrackSuccess(t *testing.T) {
	events := newMockEvents()
	config := &Config{}
	tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), nil)
	assert.NoError(t, tracker.TrackSuccess())

	runId := extractRunId(t, events)
	expectedEvents := []trackEvent{
		{
			name:        "$ld:ai:generation:success",
			context:     ldcontext.New("key"),
			metricValue: 1.0,
			data:        makeTrackData("key", "variationKey", 1, config, runId),
		},
	}

	assert.ElementsMatch(t, expectedEvents, events.events)
}

func TestTracker_TrackError(t *testing.T) {
	events := newMockEvents()
	config := &Config{}
	tracker := newTracker("key", newRunID(), "variationKey", 2, events, config, ldcontext.New("key"), nil)
	assert.NoError(t, tracker.TrackError())

	runId := extractRunId(t, events)
	expectedEvents := []trackEvent{
		{
			name:        "$ld:ai:generation:error",
			context:     ldcontext.New("key"),
			metricValue: 1.0,
			data:        makeTrackData("key", "variationKey", 2, config, runId),
		},
	}

	assert.ElementsMatch(t, expectedEvents, events.events)
}

func TestTracker_TrackRequest(t *testing.T) {
	events := newMockEvents()
	config := &Config{}
	tracker := newTracker("key", newRunID(), "variationKey", 3, events, config, ldcontext.New("key"), nil)

	expectedResponse := ProviderResponse{
		Usage: TokenUsage{
			Total: 1,
		},
		Metrics: Metrics{
			Latency:          10 * time.Millisecond,
			TimeToFirstToken: 42 * time.Millisecond,
		},
	}

	r, err := tracker.TrackRequest(func(c *Config) (ProviderResponse, error) {
		return expectedResponse, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, r)

	runId := extractRunId(t, events)
	expectedEvents := []trackEvent{
		{
			name:        "$ld:ai:generation:success",
			context:     ldcontext.New("key"),
			metricValue: 1,
			data:        makeTrackData("key", "variationKey", 3, config, runId),
		},
		{
			name:        "$ld:ai:duration:total",
			context:     ldcontext.New("key"),
			metricValue: 10.0,
			data:        makeTrackData("key", "variationKey", 3, config, runId),
		},
		{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 1,
			data:        makeTrackData("key", "variationKey", 3, config, runId),
		},
		{
			name:        "$ld:ai:tokens:ttf",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "variationKey", 3, config, runId),
		},
	}

	assert.ElementsMatch(t, expectedEvents, events.events)
}

func TestTracker_TrackRequestReceivesConfig(t *testing.T) {
	events := newMockEvents()

	expectedConfig := NewConfig().
		WithMessage("hello", datamodel.Assistant).
		WithModelName("model").
		WithProviderName("provider").
		WithModelParam("param", ldvalue.String("value")).
		WithCustomModelParam("custom", ldvalue.String("value")).
		Enable().
		Build()

	tracker := newTracker("key", newRunID(), "variationKey", 4, events, &expectedConfig, ldcontext.New("key"), nil)

	var gotConfig *Config
	_, _ = tracker.TrackRequest(func(c *Config) (ProviderResponse, error) {
		gotConfig = c
		return ProviderResponse{}, nil
	})

	assert.Equal(t, expectedConfig, *gotConfig)
}

type mockStopwatch time.Duration

func (m mockStopwatch) Start() {}

func (m mockStopwatch) Stop() time.Duration {
	return time.Duration(m)
}

func TestTracker_LatencyMeasuredIfNotProvided(t *testing.T) {
	events := newMockEvents()
	config := &Config{}

	tracker := newTrackerWithStopwatch(
		"key", newRunID(), "variationKey", 5, events, config, ldcontext.New("key"), nil, mockStopwatch(42*time.Millisecond))

	expectedResponse := ProviderResponse{
		Usage: TokenUsage{
			Total: 1,
		},
	}

	r, err := tracker.TrackRequest(func(c *Config) (ProviderResponse, error) {
		return expectedResponse, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, r)

	require.Equal(t, 3, len(events.events))
	gotEvent := events.events[1]
	assert.Equal(t, "$ld:ai:duration:total", gotEvent.name)
	assert.Equal(t, 42.0, gotEvent.metricValue)
}

func TestTracker_TrackDuration(t *testing.T) {
	events := newMockEvents()
	config := &Config{}
	tracker := newTracker("key", newRunID(), "variationKey", 6, events, config, ldcontext.New("key"), nil)

	assert.NoError(t, tracker.TrackDuration(time.Millisecond*10))

	runId := extractRunId(t, events)
	expectedEvent := trackEvent{
		name:        "$ld:ai:duration:total",
		context:     ldcontext.New("key"),
		metricValue: 10.0,
		data:        makeTrackData("key", "variationKey", 6, config, runId),
	}

	assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
}

func TestTracker_TrackFeedback(t *testing.T) {
	t.Run("positive feedback", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 7, events, config, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackFeedback(FeedbackPositive))

		runId := extractRunId(t, events)
		expectedEvent := trackEvent{
			name:        "$ld:ai:feedback:user:positive",
			context:     ldcontext.New("key"),
			metricValue: 1.0,
			data:        makeTrackData("key", "variationKey", 7, config, runId),
		}

		assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
	})

	t.Run("negative feedback", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 7, events, config, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackFeedback(FeedbackNegative))

		runId := extractRunId(t, events)
		expectedEvent := trackEvent{
			name:        "$ld:ai:feedback:user:negative",
			context:     ldcontext.New("key"),
			metricValue: 1.0,
			data:        makeTrackData("key", "variationKey", 7, config, runId),
		}

		assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
	})

	t.Run("invalid feedback returns error", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 7, events, config, ldcontext.New("key"), nil)

		assert.Error(t, tracker.TrackFeedback("not a valid feedback value"))
		assert.Empty(t, events.events)
	})
}

func TestTracker_TrackUsage(t *testing.T) {
	t.Run("only one field set, only one event", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 8, events, config, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackUsage(TokenUsage{
			Total: 42,
		}))

		runId := extractRunId(t, events)
		expectedEvent := trackEvent{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "variationKey", 8, config, runId),
		}

		assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
	})

	t.Run("all fields set, all events", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 9, events, config, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackUsage(TokenUsage{
			Total:  42,
			Input:  20,
			Output: 22,
		}))

		runId := extractRunId(t, events)
		expectedTotal := trackEvent{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "variationKey", 9, config, runId),
		}

		expectedInput := trackEvent{
			name:        "$ld:ai:tokens:input",
			context:     ldcontext.New("key"),
			metricValue: 20.0,
			data:        makeTrackData("key", "variationKey", 9, config, runId),
		}

		expectedOutput := trackEvent{
			name:        "$ld:ai:tokens:output",
			context:     ldcontext.New("key"),
			metricValue: 22.0,
			data:        makeTrackData("key", "variationKey", 9, config, runId),
		}

		assert.ElementsMatch(t, []trackEvent{expectedTotal, expectedInput, expectedOutput}, events.events)
	})
}

func TestTracker_GetSummary(t *testing.T) {
	t.Run("empty summary when nothing tracked", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", newRunID(), "variationKey", 10, events, &Config{}, ldcontext.New("key"), nil)

		summary := tracker.GetSummary()

		assert.True(t, summary.Duration.IsNone())
		assert.True(t, summary.Feedback.IsNone())
		assert.True(t, summary.Tokens.IsNone())
		assert.True(t, summary.Success.IsNone())
		assert.True(t, summary.TimeToFirstToken.IsNone())
	})

	t.Run("first duration is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", newRunID(), "variationKey", 11, events, &Config{}, ldcontext.New("key"), events.log.Loggers)

		_ = tracker.TrackDuration(time.Millisecond * 10)
		_ = tracker.TrackDuration(time.Millisecond * 20)

		summary := tracker.GetSummary()

		assert.True(t, summary.Duration.IsSome())
		assert.Equal(t, time.Millisecond*10, summary.Duration.Unwrap())
	})

	t.Run("first feedback is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", newRunID(), "variationKey", 12, events, &Config{}, ldcontext.New("key"), events.log.Loggers)

		_ = tracker.TrackFeedback(FeedbackPositive)
		_ = tracker.TrackFeedback(FeedbackNegative)

		summary := tracker.GetSummary()

		assert.True(t, summary.Feedback.IsSome())
		assert.Equal(t, FeedbackPositive, summary.Feedback.Unwrap())
	})

	t.Run("success status tracked correctly", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", newRunID(), "variationKey", 13, events, &Config{}, ldcontext.New("key"), nil)

		_ = tracker.TrackSuccess()

		summary := tracker.GetSummary()

		assert.True(t, summary.Success.IsSome())
		assert.True(t, summary.Success.Unwrap())
	})

	t.Run("time to first token is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", newRunID(), "variationKey", 14, events, &Config{}, ldcontext.New("key"), nil)

		duration := time.Millisecond * 30
		_ = tracker.TrackTimeToFirstToken(duration)

		summary := tracker.GetSummary()

		assert.True(t, summary.TimeToFirstToken.IsSome())
		assert.Equal(t, duration, summary.TimeToFirstToken.Unwrap())
	})

	t.Run("token usage is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", newRunID(), "variationKey", 15, events, &Config{}, ldcontext.New("key"), nil)

		usage := TokenUsage{
			Total:  100,
			Input:  40,
			Output: 60,
		}
		_ = tracker.TrackUsage(usage)

		summary := tracker.GetSummary()

		assert.True(t, summary.Tokens.IsSome())
		assert.Equal(t, usage, summary.Tokens.Unwrap())
	})
}

func TestTracker_RunIdPresentInTrackData(t *testing.T) {
	events := newMockEvents()
	config := &Config{}
	tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), nil)
	_ = tracker.TrackSuccess()

	require.NotEmpty(t, events.events)
	data := events.events[0].data
	runId := data.GetByKey("runId").StringValue()
	assert.NotEmpty(t, runId, "runId should be present and non-empty in track data")
}

func TestTracker_AtMostOnce(t *testing.T) {
	t.Run("TrackDuration only tracks once", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), events.log.Loggers)

		assert.NoError(t, tracker.TrackDuration(10*time.Millisecond))
		assert.NoError(t, tracker.TrackDuration(20*time.Millisecond))

		count := 0
		for _, e := range events.events {
			if e.name == "$ld:ai:duration:total" {
				count++
			}
		}
		assert.Equal(t, 1, count, "TrackDuration should only emit one event")
	})

	t.Run("TrackTimeToFirstToken only tracks once", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), events.log.Loggers)

		assert.NoError(t, tracker.TrackTimeToFirstToken(10*time.Millisecond))
		assert.NoError(t, tracker.TrackTimeToFirstToken(20*time.Millisecond))

		count := 0
		for _, e := range events.events {
			if e.name == "$ld:ai:tokens:ttf" {
				count++
			}
		}
		assert.Equal(t, 1, count, "TrackTimeToFirstToken should only emit one event")
	})

	t.Run("TrackUsage only tracks once", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), events.log.Loggers)

		assert.NoError(t, tracker.TrackUsage(TokenUsage{Total: 10}))
		assert.NoError(t, tracker.TrackUsage(TokenUsage{Total: 20}))

		count := 0
		for _, e := range events.events {
			if e.name == "$ld:ai:tokens:total" {
				count++
			}
		}
		assert.Equal(t, 1, count, "TrackUsage should only emit one event")
	})

	t.Run("TrackFeedback only tracks once", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), events.log.Loggers)

		assert.NoError(t, tracker.TrackFeedback(FeedbackPositive))
		assert.NoError(t, tracker.TrackFeedback(FeedbackNegative))

		count := 0
		for _, e := range events.events {
			if e.name == "$ld:ai:feedback:user:positive" || e.name == "$ld:ai:feedback:user:negative" {
				count++
			}
		}
		assert.Equal(t, 1, count, "TrackFeedback should only emit one event")
	})

	t.Run("TrackSuccess only tracks once", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), events.log.Loggers)

		assert.NoError(t, tracker.TrackSuccess())
		assert.NoError(t, tracker.TrackSuccess())

		count := 0
		for _, e := range events.events {
			if e.name == "$ld:ai:generation:success" {
				count++
			}
		}
		assert.Equal(t, 1, count, "TrackSuccess should only emit one event")
	})

	t.Run("TrackError only tracks once", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), events.log.Loggers)

		assert.NoError(t, tracker.TrackError())
		assert.NoError(t, tracker.TrackError())

		count := 0
		for _, e := range events.events {
			if e.name == "$ld:ai:generation:error" {
				count++
			}
		}
		assert.Equal(t, 1, count, "TrackError should only emit one event")
	})

	t.Run("TrackSuccess then TrackError only tracks success", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("key", newRunID(), "variationKey", 1, events, config, ldcontext.New("key"), events.log.Loggers)

		assert.NoError(t, tracker.TrackSuccess())
		assert.NoError(t, tracker.TrackError())

		assert.Equal(t, 1, len(events.events))
		assert.Equal(t, "$ld:ai:generation:success", events.events[0].name)
	})
}

func TestTracker_ResumptionToken(t *testing.T) {
	t.Run("produces valid base64url-encoded token", func(t *testing.T) {
		events := newMockEvents()
		config := &Config{}
		tracker := newTracker("my-config", newRunID(), "var-1", 3, events, config, ldcontext.New("key"), nil)

		token := tracker.ResumptionToken()
		assert.NotEmpty(t, token)

		// Decode and verify
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err)

		var payload struct {
			RunID        string `json:"runId"`
			ConfigKey    string `json:"configKey"`
			VariationKey string `json:"variationKey"`
			Version      int    `json:"version"`
		}
		require.NoError(t, json.Unmarshal(decoded, &payload))

		assert.NotEmpty(t, payload.RunID)
		assert.Equal(t, "my-config", payload.ConfigKey)
		assert.Equal(t, "var-1", payload.VariationKey)
		assert.Equal(t, 3, payload.Version)
	})

	t.Run("does not include modelName or providerName", func(t *testing.T) {
		events := newMockEvents()
		config := NewConfig().WithModelName("gpt-4").WithProviderName("openai").Build()
		tracker := newTracker("key", newRunID(), "var", 1, events, &config, ldcontext.New("key"), nil)

		token := tracker.ResumptionToken()
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err)

		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(decoded, &raw))

		_, hasModel := raw["modelName"]
		_, hasProvider := raw["providerName"]
		assert.False(t, hasModel, "token should not contain modelName")
		assert.False(t, hasProvider, "token should not contain providerName")
	})
}

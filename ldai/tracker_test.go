package ldai

import (
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
		newTracker("key", "variationKey", 1, newMockEvents(), nil, ldcontext.New("key"), nil)
	})
}

func TestTracker_NewDoesNotPanicWithConfig(t *testing.T) {
	assert.NotPanics(t, func() {
		newTracker("key", "variationKey", 1, newMockEvents(), &Config{}, ldcontext.New("key"), nil)
	})
}

func makeTrackData(configKey, variationKey string, version int) ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("variationKey", ldvalue.String(variationKey)).
		Set("configKey", ldvalue.String(configKey)).
		Set("version", ldvalue.Int(version)).
		Build()
}

func TestTracker_TrackSuccess(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "variationKey", 1, events, &Config{}, ldcontext.New("key"), nil)
	assert.NoError(t, tracker.TrackSuccess())

	expectedEvents := []trackEvent{
		{
			name:        "$ld:ai:generation:success",
			context:     ldcontext.New("key"),
			metricValue: 1.0,
			data:        makeTrackData("key", "variationKey", 1),
		},
	}

	assert.ElementsMatch(t, expectedEvents, events.events)
}

func TestTracker_TrackError(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "variationKey", 2, events, &Config{}, ldcontext.New("key"), nil)
	assert.NoError(t, tracker.TrackError())

	expectedEvents := []trackEvent{
		{
			name:        "$ld:ai:generation:error",
			context:     ldcontext.New("key"),
			metricValue: 1.0,
			data:        makeTrackData("key", "variationKey", 2),
		},
	}

	assert.ElementsMatch(t, expectedEvents, events.events)
}

func TestTracker_TrackRequest(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "variationKey", 3, events, &Config{}, ldcontext.New("key"), nil)

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

	expectedEvents := []trackEvent{
		{
			name:        "$ld:ai:generation:success",
			context:     ldcontext.New("key"),
			metricValue: 1,
			data:        makeTrackData("key", "variationKey", 3),
		},
		{
			name:        "$ld:ai:duration:total",
			context:     ldcontext.New("key"),
			metricValue: 10.0,
			data:        makeTrackData("key", "variationKey", 3),
		},
		{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 1,
			data:        makeTrackData("key", "variationKey", 3),
		},
		{
			name:        "$ld:ai:tokens:ttf",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "variationKey", 3),
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

	tracker := newTracker("key", "variationKey", 4, events, &expectedConfig, ldcontext.New("key"), nil)

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

	tracker := newTrackerWithStopwatch(
		"key", "variationKey", 5, events, &Config{}, ldcontext.New("key"), nil, mockStopwatch(42*time.Millisecond))

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
	gotEvent := events.events[2]
	assert.Equal(t, "$ld:ai:duration:total", gotEvent.name)
	assert.Equal(t, 42.0, gotEvent.metricValue)
}

func TestTracker_TrackDuration(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "variationKey", 6, events, &Config{}, ldcontext.New("key"), nil)

	assert.NoError(t, tracker.TrackDuration(time.Millisecond*10))

	expectedEvent := trackEvent{
		name:        "$ld:ai:duration:total",
		context:     ldcontext.New("key"),
		metricValue: 10.0,
		data:        makeTrackData("key", "variationKey", 6),
	}

	assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
}

func TestTracker_TrackFeedback(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "variationKey", 7, events, &Config{}, ldcontext.New("key"), nil)

	assert.NoError(t, tracker.TrackFeedback(FeedbackPositive))
	assert.NoError(t, tracker.TrackFeedback(FeedbackNegative))
	assert.Error(t, tracker.TrackFeedback("not a valid feedback value"))

	expectedPositiveEvent := trackEvent{
		name:        "$ld:ai:feedback:user:positive",
		context:     ldcontext.New("key"),
		metricValue: 1.0,
		data:        makeTrackData("key", "variationKey", 7),
	}

	expectedNegativeEvent := trackEvent{
		name:        "$ld:ai:feedback:user:negative",
		context:     ldcontext.New("key"),
		metricValue: 1.0,
		data:        makeTrackData("key", "variationKey", 7),
	}

	assert.ElementsMatch(t, []trackEvent{expectedPositiveEvent, expectedNegativeEvent}, events.events)
}

func TestTracker_TrackUsage(t *testing.T) {
	t.Run("only one field set, only one event", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 8, events, &Config{}, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackUsage(TokenUsage{
			Total: 42,
		}))

		expectedEvent := trackEvent{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "variationKey", 8),
		}

		assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
	})

	t.Run("all fields set, all events", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 9, events, &Config{}, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackUsage(TokenUsage{
			Total:  42,
			Input:  20,
			Output: 22,
		}))

		expectedTotal := trackEvent{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "variationKey", 9),
		}

		expectedInput := trackEvent{
			name:        "$ld:ai:tokens:input",
			context:     ldcontext.New("key"),
			metricValue: 20.0,
			data:        makeTrackData("key", "variationKey", 9),
		}

		expectedOutput := trackEvent{
			name:        "$ld:ai:tokens:output",
			context:     ldcontext.New("key"),
			metricValue: 22.0,
			data:        makeTrackData("key", "variationKey", 9),
		}

		assert.ElementsMatch(t, []trackEvent{expectedTotal, expectedInput, expectedOutput}, events.events)
	})
}

func TestTracker_GetSummary(t *testing.T) {
	t.Run("empty summary when nothing tracked", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 10, events, &Config{}, ldcontext.New("key"), nil)

		summary := tracker.GetSummary()

		assert.True(t, summary.Duration.IsNone())
		assert.True(t, summary.Feedback.IsNone())
		assert.True(t, summary.Tokens.IsNone())
		assert.True(t, summary.Success.IsNone())
		assert.True(t, summary.TimeToFirstToken.IsNone())
	})

	t.Run("latest duration is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 11, events, &Config{}, ldcontext.New("key"), nil)

		_ = tracker.TrackDuration(time.Millisecond * 10)
		_ = tracker.TrackDuration(time.Millisecond * 20)

		summary := tracker.GetSummary()

		assert.True(t, summary.Duration.IsSome())
		assert.Equal(t, time.Millisecond*20, summary.Duration.Unwrap())
	})

	t.Run("latest feedback is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 12, events, &Config{}, ldcontext.New("key"), nil)

		_ = tracker.TrackFeedback(FeedbackPositive)
		_ = tracker.TrackFeedback(FeedbackNegative)

		summary := tracker.GetSummary()

		assert.True(t, summary.Feedback.IsSome())
		assert.Equal(t, FeedbackNegative, summary.Feedback.Unwrap())
	})

	t.Run("success status tracked correctly", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 13, events, &Config{}, ldcontext.New("key"), nil)

		_ = tracker.TrackSuccess()

		summary := tracker.GetSummary()

		assert.True(t, summary.Success.IsSome())
		assert.True(t, summary.Success.Unwrap())

		_ = tracker.TrackError()

		summary = tracker.GetSummary()

		assert.True(t, summary.Success.IsSome())
		assert.False(t, summary.Success.Unwrap())
	})

	t.Run("time to first token is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 14, events, &Config{}, ldcontext.New("key"), nil)

		duration := time.Millisecond * 30
		_ = tracker.TrackTimeToFirstToken(duration)

		summary := tracker.GetSummary()

		assert.True(t, summary.TimeToFirstToken.IsSome())
		assert.Equal(t, duration, summary.TimeToFirstToken.Unwrap())
	})

	t.Run("token usage is returned", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "variationKey", 15, events, &Config{}, ldcontext.New("key"), nil)

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

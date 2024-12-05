package ldai

import (
	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		newTracker("key", "versionKey", newMockEvents(), nil, ldcontext.New("key"), nil)
	})
}

func TestTracker_NewDoesNotPanicWithConfig(t *testing.T) {
	assert.NotPanics(t, func() {
		newTracker("key", "versionKey", newMockEvents(), &Config{}, ldcontext.New("key"), nil)
	})
}

func makeTrackData(configKey, versionKey string) ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("versionKey", ldvalue.String(versionKey)).
		Set("configKey", ldvalue.String(configKey)).Build()
}

func TestTracker_TrackSuccess(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "versionKey", events, &Config{}, ldcontext.New("key"), nil)
	assert.NoError(t, tracker.TrackSuccess())

	expectedEvent := trackEvent{
		name:        "$ld:ai:generation",
		context:     ldcontext.New("key"),
		metricValue: 1.0,
		data:        makeTrackData("key", "versionKey"),
	}

	assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
}

func TestTracker_TrackRequest(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "versionKey", events, &Config{}, ldcontext.New("key"), nil)

	expectedResponse := ProviderResponse{
		Usage: TokenUsage{
			Total: 1,
		},
		Metrics: Metrics{
			Latency: 10 * time.Millisecond,
		},
	}

	r, err := tracker.TrackRequest(func(c *Config) (ProviderResponse, error) {
		return expectedResponse, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, r)

	expectedSuccessEvent := trackEvent{
		name:        "$ld:ai:generation",
		context:     ldcontext.New("key"),
		metricValue: 1,
		data:        makeTrackData("key", "versionKey"),
	}

	expectedDurationEvent := trackEvent{
		name:        "$ld:ai:duration:total",
		context:     ldcontext.New("key"),
		metricValue: 10.0,
		data:        makeTrackData("key", "versionKey"),
	}

	expectedTokenUsageEvent := trackEvent{
		name:        "$ld:ai:tokens:total",
		context:     ldcontext.New("key"),
		metricValue: 1,
		data:        makeTrackData("key", "versionKey"),
	}

	expectedEvents := []trackEvent{expectedSuccessEvent, expectedDurationEvent, expectedTokenUsageEvent}
	assert.ElementsMatch(t, expectedEvents, events.events)
}

func TestTracker_TrackRequestReceivesConfig(t *testing.T) {
	events := newMockEvents()

	expectedConfig := &Config{
		c: datamodel.Config{
			Messages: []datamodel.Message{
				{
					Content: "hello",
					Role:    datamodel.Assistant,
				},
			},
			Meta: datamodel.Meta{
				VersionKey: "version",
				Enabled:    true,
			},
			Model: datamodel.Model{
				Id: "model",
				Parameters: map[string]ldvalue.Value{
					"param": ldvalue.String("value"),
				},
				Custom: map[string]ldvalue.Value{
					"custom": ldvalue.String("value"),
				},
			},
			Provider: datamodel.Provider{
				Id: "provider",
			},
		},
	}

	tracker := newTracker("key", events, expectedConfig, ldcontext.New("key"), nil)

	var gotConfig *Config
	_, _ = tracker.TrackRequest(func(c *Config) (ProviderResponse, error) {
		gotConfig = c
		return ProviderResponse{}, nil
	})

	assert.Equal(t, expectedConfig, gotConfig)
}

type mockStopwatch time.Duration

func (m mockStopwatch) Start() {}

func (m mockStopwatch) Stop() time.Duration {
	return time.Duration(m)
}

func TestTracker_LatencyMeasuredIfNotProvided(t *testing.T) {
	events := newMockEvents()

	tracker := newTrackerWithStopwatch(
		"key", "versionKey", events, &Config{}, ldcontext.New("key"), nil, mockStopwatch(42*time.Millisecond))

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
	tracker := newTracker("key", "versionKey", events, &Config{}, ldcontext.New("key"), nil)

	assert.NoError(t, tracker.TrackDuration(time.Millisecond*10))

	expectedEvent := trackEvent{
		name:        "$ld:ai:duration:total",
		context:     ldcontext.New("key"),
		metricValue: 10.0,
		data:        makeTrackData("key", "versionKey"),
	}

	assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
}

func TestTracker_TrackFeedback(t *testing.T) {
	events := newMockEvents()
	tracker := newTracker("key", "versionKey", events, &Config{}, ldcontext.New("key"), nil)

	assert.NoError(t, tracker.TrackFeedback(Positive))
	assert.NoError(t, tracker.TrackFeedback(Negative))
	assert.Error(t, tracker.TrackFeedback("not a valid feedback value"))

	expectedPositiveEvent := trackEvent{
		name:        "$ld:ai:feedback:user:positive",
		context:     ldcontext.New("key"),
		metricValue: 1.0,
		data:        makeTrackData("key", "versionKey"),
	}

	expectedNegativeEvent := trackEvent{
		name:        "$ld:ai:feedback:user:negative",
		context:     ldcontext.New("key"),
		metricValue: 1.0,
		data:        makeTrackData("key", "versionKey"),
	}

	assert.ElementsMatch(t, []trackEvent{expectedPositiveEvent, expectedNegativeEvent}, events.events)
}

func TestTracker_TrackUsage(t *testing.T) {
	t.Run("only one field set, only one event", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "versionKey", events, &Config{}, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackUsage(TokenUsage{
			Total: 42,
		}))

		expectedEvent := trackEvent{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "versionKey"),
		}

		assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
	})

	t.Run("all fields set, all events", func(t *testing.T) {
		events := newMockEvents()
		tracker := newTracker("key", "versionKey", events, &Config{}, ldcontext.New("key"), nil)

		assert.NoError(t, tracker.TrackUsage(TokenUsage{
			Total:  42,
			Input:  20,
			Output: 22,
		}))

		expectedTotal := trackEvent{
			name:        "$ld:ai:tokens:total",
			context:     ldcontext.New("key"),
			metricValue: 42.0,
			data:        makeTrackData("key", "versionKey"),
		}

		expectedInput := trackEvent{
			name:        "$ld:ai:tokens:input",
			context:     ldcontext.New("key"),
			metricValue: 20.0,
			data:        makeTrackData("key", "versionKey"),
		}

		expectedOutput := trackEvent{
			name:        "$ld:ai:tokens:output",
			context:     ldcontext.New("key"),
			metricValue: 22.0,
			data:        makeTrackData("key", "versionKey"),
		}

		assert.ElementsMatch(t, []trackEvent{expectedTotal, expectedInput, expectedOutput}, events.events)
	})
}

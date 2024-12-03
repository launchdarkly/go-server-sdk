package ldai

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/stretchr/testify/assert"
	"testing"
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
		NewTracker("key", newMockEvents(), nil, ldcontext.New("key"), nil)
	})
}

func TestTracker_NewDoesNotPanicWithConfig(t *testing.T) {
	assert.NotPanics(t, func() {
		NewTracker("key", newMockEvents(), &Config{}, ldcontext.New("key"), nil)
	})
}

func makeTrackData(configKey, versionKey string) ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("versionKey", ldvalue.String(versionKey)).
		Set("configKey", ldvalue.String(configKey)).Build()
}

func TestTracker_TrackSuccess(t *testing.T) {
	events := newMockEvents()
	tracker := NewTracker("key", events, &Config{}, ldcontext.New("key"), nil)
	assert.NoError(t, tracker.TrackSuccess())

	expectedEvent := trackEvent{
		name:        "$ld:ai:generation",
		context:     ldcontext.New("key"),
		metricValue: 1.0,
		data:        makeTrackData("key", ""),
	}

	assert.ElementsMatch(t, []trackEvent{expectedEvent}, events.events)
}

func TestTracker_TrackRequest(t *testing.T) {
	events := newMockEvents()
	tracker := NewTracker("key", events, &Config{}, ldcontext.New("key"), nil)

	expectedResponse := ProviderResponse{
		Usage: TokenUsage{
			Total: 1,
		},
		Metrics: Metrics{
			LatencyMs: 1.0,
		},
	}

	r, err := tracker.TrackRequest(func() (ProviderResponse, error) {
		return expectedResponse, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, expectedResponse, r)

	expectedSuccessEvent := trackEvent{
		name:        "$ld:ai:generation",
		context:     ldcontext.New("key"),
		metricValue: 1,
		data:        makeTrackData("key", ""),
	}

	expectedDurationEvent := trackEvent{
		name:        "$ld:ai:duration:total",
		context:     ldcontext.New("key"),
		metricValue: 1.0,
		data:        makeTrackData("key", ""),
	}

	expectedTokenUsageEvent := trackEvent{
		name:        "$ld:ai:tokens:total",
		context:     ldcontext.New("key"),
		metricValue: 1,
		data:        makeTrackData("key", ""),
	}

	expectedEvents := []trackEvent{expectedSuccessEvent, expectedDurationEvent, expectedTokenUsageEvent}
	assert.ElementsMatch(t, expectedEvents, events.events)
}

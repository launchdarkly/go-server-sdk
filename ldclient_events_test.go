package ldclient

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	helpers "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentifySendsIdentifyEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	err := client.Identify(user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.IdentifyEventData)
	assert.Equal(t, ldevents.Context(user), e.Context)
	assert.Equal(t, ldvalue.NewOptionalInt(1), e.SamplingRatio)
}

func TestIdentifyWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Identify(lduser.NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestTrackEventSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	err := client.TrackEvent(key, user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEventData)
	assert.Equal(t, ldevents.Context(user), e.Context)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, ldvalue.Null(), e.Data)
	assert.False(t, e.HasMetric)
	assert.Equal(t, ldvalue.NewOptionalInt(1), e.SamplingRatio)
}

func TestTrackEventSendsSamplingRatio(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"

	err := client.TrackEvent(key, user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEventData)
	assert.Equal(t, ldevents.Context(user), e.Context)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, ldvalue.Null(), e.Data)
	assert.False(t, e.HasMetric)
	assert.Equal(t, ldvalue.NewOptionalInt(1), e.SamplingRatio)
}

func TestTrackDataSendsCustomEventWithData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	data := ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))
	err := client.TrackData(key, user, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEventData)
	assert.Equal(t, ldevents.Context(user), e.Context)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
	assert.False(t, e.HasMetric)
}

func TestTrackMetricSendsCustomEventWithMetricAndData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	data := ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))
	metric := float64(1.5)
	err := client.TrackMetric(key, user, metric, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEventData)
	assert.Equal(t, ldevents.Context(user), e.Context)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
	assert.True(t, e.HasMetric)
	assert.Equal(t, metric, e.MetricValue)
}

func TestTrackWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.TrackEvent("eventkey", lduser.NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestTrackMetricWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.TrackMetric("eventKey", lduser.NewUser(""), 2.5, ldvalue.Null())
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*mocks.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestIdentifyWithEventsDisabledDoesNotCauseError(t *testing.T) {
	mockLog := ldlogtest.NewMockLog()
	client := makeTestClientWithConfig(func(c *Config) {
		c.Events = ldcomponents.NoEvents()
		c.Logging = ldcomponents.Logging().Loggers(mockLog.Loggers)
	})
	defer client.Close()

	require.NoError(t, client.Identify(lduser.NewUser("")))

	assert.Len(t, mockLog.GetOutput(ldlog.Warn), 0)
}

func TestTrackWithEventsDisabledDoesNotCauseError(t *testing.T) {
	mockLog := ldlogtest.NewMockLog()
	client := makeTestClientWithConfig(func(c *Config) {
		c.Events = ldcomponents.NoEvents()
		c.Logging = ldcomponents.Logging().Loggers(mockLog.Loggers)
	})
	defer client.Close()

	require.NoError(t, client.TrackEvent("eventkey", lduser.NewUser("")))
	require.NoError(t, client.TrackMetric("eventkey", lduser.NewUser(""), 0, ldvalue.Null()))

	assert.Len(t, mockLog.GetOutput(ldlog.Warn), 0)
}

func TestWithEventsDisabledDecorator(t *testing.T) {
	doTest := func(name string, fn func(*LDClient) interfaces.LDClientInterface, shouldBeSent bool) {
		t.Run(name, func(t *testing.T) {
			events := &mocks.CapturingEventProcessor{}
			config := Config{
				DataSource: ldcomponents.ExternalUpdatesOnly(),
				Events:     mocks.SingleComponentConfigurer[ldevents.EventProcessor]{Instance: events},
			}
			client, err := MakeCustomClient("", config, 0)
			require.NoError(t, err)

			ci := fn(client)
			checkEvents := func(action func()) {
				events.Events = nil
				action()
				if shouldBeSent {
					assert.Len(t, events.Events, 1, "should have recorded an event, but did not")
				} else {
					assert.Len(t, events.Events, 0, "should not have recorded an event, but did")
				}
			}
			user := lduser.NewUser("userkey")
			checkEvents(func() { _, _ = ci.BoolVariation("flagkey", user, false) })
			checkEvents(func() { _, _, _ = ci.BoolVariationDetail("flagkey", user, false) })
			checkEvents(func() { _, _ = ci.IntVariation("flagkey", user, 0) })
			checkEvents(func() { _, _, _ = ci.IntVariationDetail("flagkey", user, 0) })
			checkEvents(func() { _, _ = ci.Float64Variation("flagkey", user, 0) })
			checkEvents(func() { _, _, _ = ci.Float64VariationDetail("flagkey", user, 0) })
			checkEvents(func() { _, _ = ci.StringVariation("flagkey", user, "") })
			checkEvents(func() { _, _, _ = ci.StringVariationDetail("flagkey", user, "") })
			checkEvents(func() { _, _ = ci.JSONVariation("flagkey", user, ldvalue.Null()) })
			checkEvents(func() { _, _, _ = ci.JSONVariationDetail("flagkey", user, ldvalue.Null()) })
			checkEvents(func() { ci.Identify(user) })
			checkEvents(func() { ci.TrackEvent("eventkey", user) })
			checkEvents(func() { ci.TrackData("eventkey", user, ldvalue.Bool(true)) })
			checkEvents(func() { ci.TrackMetric("eventkey", user, 1.5, ldvalue.Null()) })

			state := ci.AllFlagsState(user)
			assert.True(t, state.IsValid())
		})
	}

	doTest("client.WithEventsDisabled(false)",
		func(c *LDClient) interfaces.LDClientInterface { return c.WithEventsDisabled(false) },
		true)

	doTest("client.WithEventsDisabled(true)",
		func(c *LDClient) interfaces.LDClientInterface { return c.WithEventsDisabled(true) },
		false)

	doTest("client.WithEventsDisabled(true).WithEventsDisabled(false)",
		func(c *LDClient) interfaces.LDClientInterface {
			return c.WithEventsDisabled(true).WithEventsDisabled(false)
		},
		true)

	doTest("client.WithEventsDisabled(true).WithEventsDisabled(true)",
		func(c *LDClient) interfaces.LDClientInterface {
			return c.WithEventsDisabled(true).WithEventsDisabled(true)
		},
		false)
}

func TestFlushAsync(t *testing.T) {
	g := newGatedEventSender()
	client := makeTestClientWithEventSender(g)
	defer client.Close()

	client.Identify(evalTestUser)
	client.Flush()

	helpers.AssertNoMoreValues(t, g.didSendCh, time.Millisecond*50) // didn't do the flush yet

	g.canSendCh <- struct{}{} // allow the sender to proceed with the fake flush

	helpers.RequireValue(t, g.didSendCh, time.Millisecond*100) // now the flush has happened
}

func TestFlushAndWaitSucceeds(t *testing.T) {
	g := newGatedEventSender()
	client := makeTestClientWithEventSender(g)
	defer client.Close()

	client.Identify(evalTestUser)

	go func() {
		time.Sleep(time.Millisecond * 20)
		g.canSendCh <- struct{}{}
	}()

	sent := client.FlushAndWait(time.Millisecond * 500)
	assert.True(t, sent)

	helpers.RequireValue(t, g.didSendCh, time.Millisecond*50)
}

func TestFlushAndWaitTimesOut(t *testing.T) {
	g := newGatedEventSender()
	client := makeTestClientWithEventSender(g)
	defer client.Close()

	client.Identify(evalTestUser)

	go func() {
		time.Sleep(time.Millisecond * 200)
		g.canSendCh <- struct{}{}
	}()

	sent := client.FlushAndWait(time.Millisecond * 10)
	assert.False(t, sent)

	helpers.AssertNoMoreValues(t, g.didSendCh, time.Millisecond*50) // didn't do the flush yet
}

type gatedEventSender struct {
	canSendCh chan struct{}
	didSendCh chan struct{}
}

func newGatedEventSender() *gatedEventSender {
	return &gatedEventSender{canSendCh: make(chan struct{}, 100), didSendCh: make(chan struct{}, 100)}
}

func (g *gatedEventSender) SendEventData(kind ldevents.EventDataKind, data []byte, eventCount int) ldevents.EventSenderResult {
	<-g.canSendCh
	g.didSendCh <- struct{}{}
	return ldevents.EventSenderResult{Success: true}
}

func makeTestClientWithEventSender(s ldevents.EventSender) *LDClient {
	eventsConfig := ldevents.EventsConfiguration{
		Capacity:              1000,
		EventSender:           s,
		FlushInterval:         time.Hour,
		Loggers:               ldlog.NewDisabledLoggers(),
		UserKeysCapacity:      1000,
		UserKeysFlushInterval: time.Hour,
	}
	ep := ldevents.NewDefaultEventProcessor(eventsConfig)
	return makeTestClientWithConfig(func(c *Config) {
		c.Events = mocks.SingleComponentConfigurer[ldevents.EventProcessor]{Instance: ep}
	})
}

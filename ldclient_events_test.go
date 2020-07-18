package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

func TestIdentifySendsIdentifyEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	err := client.Identify(user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.IdentifyEvent)
	assert.Equal(t, ldevents.User(user), e.User)
}

func TestIdentifyWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Identify(lduser.NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestTrackEventSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	err := client.TrackEvent(key, user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEvent)
	assert.Equal(t, ldevents.User(user), e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, ldvalue.Null(), e.Data)
	assert.False(t, e.HasMetric)
}

func TestTrackDataSendsCustomEventWithData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	data := ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))
	err := client.TrackData(key, user, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEvent)
	assert.Equal(t, ldevents.User(user), e.User)
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

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEvent)
	assert.Equal(t, ldevents.User(user), e.User)
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

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestTrackMetricWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.TrackMetric("eventKey", lduser.NewUser(""), 2.5, ldvalue.Null())
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
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
			events := &sharedtest.CapturingEventProcessor{}
			config := Config{
				DataSource: ldcomponents.ExternalUpdatesOnly(),
				Events:     sharedtest.SingleEventProcessorFactory{Instance: events},
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

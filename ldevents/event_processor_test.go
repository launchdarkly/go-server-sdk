package ldevents

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
)

var epDefaultConfig = EventsConfiguration{
	Capacity:              1000,
	FlushInterval:         1 * time.Hour,
	UserKeysCapacity:      1000,
	UserKeysFlushInterval: 1 * time.Hour,
}

var epDefaultUser = lduser.NewUserBuilder("userKey").Name("Red").Build()

var userJson = ldvalue.ObjectBuild().
	Set("key", ldvalue.String("userKey")).
	Set("name", ldvalue.String("Red")).
	Build()
var filteredUserJson = ldvalue.ObjectBuild().
	Set("key", ldvalue.String("userKey")).
	Set("privateAttrs", ldvalue.ArrayOf(ldvalue.String("name"))).
	Build()

const (
	sdkKey = "SDK_KEY"
)

func TestIdentifyEventIsQueued(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	ie := defaultEventFactory.NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 1, len(es.events)) {
		assert.Equal(t, expectedIdentifyEvent(ie, userJson), es.events[0])
	}
}

func TestUserDetailsAreScrubbedInIdentifyEvent(t *testing.T) {
	config := epDefaultConfig
	config.AllAttributesPrivate = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	ie := defaultEventFactory.NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 1, len(es.events)) {
		assert.Equal(t, expectedIdentifyEvent(ie, filteredUserJson), es.events[0])
	}
}

func TestFeatureEventIsSummarizedAndNotTrackedByDefault(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	flag := ldeval.FeatureFlag{
		Key:     "flagkey",
		Version: 11,
	}
	value := ldvalue.String("value")
	fe := defaultEventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe, userJson), es.events[0])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[1])
	}
}

func TestIndividualFeatureEventIsQueuedWhenTrackEventsIsTrue(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	flag := ldeval.FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := ldvalue.String("value")
	fe := defaultEventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 3, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe, userJson), es.events[0])
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, false, nil), es.events[1])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[2])
	}
}

func TestUserDetailsAreScrubbedInIndexEvent(t *testing.T) {
	config := epDefaultConfig
	config.AllAttributesPrivate = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	flag := ldeval.FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := ldvalue.String("value")
	fe := defaultEventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 3, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe, filteredUserJson), es.events[0])
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, false, nil), es.events[1])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[2])
	}
}

func TestFeatureEventCanContainInlineUser(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	flag := ldeval.FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := ldvalue.String("value")
	fe := defaultEventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, false, &userJson), es.events[0])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[1])
	}
}

func TestUserDetailsAreScrubbedInFeatureEvent(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	config.AllAttributesPrivate = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	flag := ldeval.FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := ldvalue.String("value")
	fe := defaultEventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, false, &filteredUserJson), es.events[0])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[1])
	}
}

func TestFeatureEventCanContainReason(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	flag := ldeval.FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: true,
	}
	value := ldvalue.String("value")
	fe := defaultEventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	fe.Reason = ldreason.NewEvalReasonFallthrough()
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, false, &userJson), es.events[0])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[1])
	}
}

func TestIndexEventIsGeneratedForNonTrackedFeatureEventEvenIfInliningIsOn(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	flag := ldeval.FeatureFlag{
		Key:         "flagkey",
		Version:     11,
		TrackEvents: false,
	}
	value := ldvalue.String("value")
	fe := defaultEventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe, userJson), es.events[0]) // we get this because we are *not* getting the full event
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[1])
	}
}

func TestDebugEventIsAddedIfFlagIsTemporarilyInDebugMode(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := epDefaultConfig
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	futureTime := uint64(fakeTimeNow + 100)
	flag := ldeval.FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          false,
		DebugEventsUntilDate: &futureTime,
	}
	value := ldvalue.String("value")
	fe := eventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 3, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe, userJson), es.events[0])
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, true, &userJson), es.events[1])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[2])
	}
}

func TestEventCanBeBothTrackedAndDebugged(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := epDefaultConfig
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	futureTime := uint64(fakeTimeNow + 100)
	flag := ldeval.FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          true,
		DebugEventsUntilDate: &futureTime,
	}
	value := ldvalue.String("value")
	fe := eventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 4, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe, userJson), es.events[0])
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, false, nil), es.events[1])
		assert.Equal(t, expectedFeatureEvent(fe, flag, value, true, &userJson), es.events[2])
		assertSummaryEventHasCounter(t, flag, 2, value, 1, es.events[3])
	}
}

func TestDebugModeExpiresBasedOnClientTimeIfClientTimeIsLater(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := epDefaultConfig
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	// Pick a server time that is somewhat behind the client time
	serverTime := fakeTimeNow - 20000
	es.result = EventSenderResult{Success: true, TimeFromServer: serverTime}

	// Send and flush an event we don't care about, just to set the last server time
	ie := eventFactory.NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	// Now send an event with debug mode on, with a "debug until" time that is further in
	// the future than the server time, but in the past compared to the client.
	debugUntil := uint64(serverTime + 1000)
	flag := ldeval.FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          false,
		DebugEventsUntilDate: &debugUntil,
	}
	fe := eventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, -1, ldvalue.Null(), ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedIdentifyEvent(ie, userJson), es.events[0])
		// should get a summary event only, not a debug event
		assertSummaryEventHasCounter(t, flag, -1, ldvalue.Null(), 1, es.events[1])
	}
}

func TestDebugModeExpiresBasedOnServerTimeIfServerTimeIsLater(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := epDefaultConfig
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	// Pick a server time that is somewhat ahead of the client time
	serverTime := fakeTimeNow + 20000
	es.result = EventSenderResult{Success: true, TimeFromServer: serverTime}

	// Send and flush an event we don't care about, just to set the last server time
	ie := eventFactory.NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	// Now send an event with debug mode on, with a "debug until" time that is further in
	// the future than the client time, but in the past compared to the server.
	debugUntil := uint64(serverTime - 1000)
	flag := ldeval.FeatureFlag{
		Key:                  "flagkey",
		Version:              11,
		TrackEvents:          false,
		DebugEventsUntilDate: &debugUntil,
	}
	fe := eventFactory.NewSuccessfulEvalEvent(&flag, epDefaultUser, -1, ldvalue.Null(), ldvalue.Null(), noReason, "")
	ep.SendEvent(fe)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedIdentifyEvent(ie, userJson), es.events[0])
		// should get a summary event only, not a debug event
		assertSummaryEventHasCounter(t, flag, -1, ldvalue.Null(), 1, es.events[1])
	}
}

func TestTwoFeatureEventsForSameUserGenerateOnlyOneIndexEvent(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	flag1 := ldeval.FeatureFlag{
		Key:         "flagkey1",
		Version:     11,
		TrackEvents: true,
	}
	flag2 := ldeval.FeatureFlag{
		Key:         "flagkey2",
		Version:     22,
		TrackEvents: true,
	}
	value := ldvalue.String("value")
	fe1 := defaultEventFactory.NewSuccessfulEvalEvent(&flag1, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	fe2 := defaultEventFactory.NewSuccessfulEvalEvent(&flag2, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe1)
	ep.SendEvent(fe2)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 4, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe1, userJson), es.events[0])
		assert.Equal(t, expectedFeatureEvent(fe1, flag1, value, false, nil), es.events[1])
		assert.Equal(t, expectedFeatureEvent(fe2, flag2, value, false, nil), es.events[2])
		assertSummaryEventHasCounter(t, flag1, 2, value, 1, es.events[3])
		assertSummaryEventHasCounter(t, flag2, 2, value, 1, es.events[3])
	}
}

func TestNonTrackedEventsAreSummarized(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	flag1 := ldeval.FeatureFlag{
		Key:         "flagkey1",
		Version:     11,
		TrackEvents: false,
	}
	flag2 := ldeval.FeatureFlag{
		Key:         "flagkey2",
		Version:     22,
		TrackEvents: false,
	}
	value := ldvalue.String("value")
	fe1 := defaultEventFactory.NewSuccessfulEvalEvent(&flag1, epDefaultUser, 2, value, ldvalue.Null(), noReason, "")
	fe2 := defaultEventFactory.NewSuccessfulEvalEvent(&flag2, epDefaultUser, 3, value, ldvalue.Null(), noReason, "")
	fe3 := defaultEventFactory.NewSuccessfulEvalEvent(&flag2, epDefaultUser, 3, value, ldvalue.Null(), noReason, "")
	ep.SendEvent(fe1)
	ep.SendEvent(fe2)
	ep.SendEvent(fe3)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(fe1, userJson), es.events[0])

		se := es.events[1]
		assertSummaryEventHasCounter(t, flag1, 2, value, 1, se)
		assertSummaryEventHasCounter(t, flag2, 3, value, 2, se)
		assert.Equal(t, float64(fe1.CreationDate), se.GetByKey("startDate").Float64Value())
		assert.Equal(t, float64(fe3.CreationDate), se.GetByKey("endDate").Float64Value())
	}
}

func TestCustomEventIsQueuedWithUser(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	data := ldvalue.ObjectBuild().Set("thing", ldvalue.String("stuff")).Build()
	ce := defaultEventFactory.NewCustomEvent("eventkey", epDefaultUser, data, false, 0)
	ep.SendEvent(ce)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 2, len(es.events)) {
		assert.Equal(t, expectedIndexEvent(ce, userJson), es.events[0])

		expected := ldvalue.ObjectBuild().
			Set("kind", ldvalue.String("custom")).
			Set("creationDate", ldvalue.Float64(float64(ce.CreationDate))).
			Set("key", ldvalue.String(ce.Key)).
			Set("data", data).
			Set("userKey", ldvalue.String(epDefaultUser.GetKey())).
			Build()
		assert.Equal(t, expected, es.events[1])
	}
}

func TestCustomEventCanContainInlineUser(t *testing.T) {
	config := epDefaultConfig
	config.InlineUsersInEvents = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	data := ldvalue.ObjectBuild().Set("thing", ldvalue.String("stuff")).Build()
	ce := defaultEventFactory.NewCustomEvent("eventkey", epDefaultUser, data, false, 0)
	ep.SendEvent(ce)
	ep.Flush()
	ep.waitUntilInactive()

	if assert.Equal(t, 1, len(es.events)) {
		expected := ldvalue.ObjectBuild().
			Set("kind", ldvalue.String("custom")).
			Set("creationDate", ldvalue.Float64(float64(ce.CreationDate))).
			Set("key", ldvalue.String(ce.Key)).
			Set("data", data).
			Set("user", userJsonEncoding(epDefaultUser)).
			Build()
		assert.Equal(t, expected, es.events[0])
	}
}

func TestClosingEventProcessorForcesSynchronousFlush(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	ie := defaultEventFactory.NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Close()

	if assert.Equal(t, 1, len(es.events)) {
		assert.Equal(t, expectedIdentifyEvent(ie, userJson), es.events[0])
	}
}

func TestNothingIsSentIfThereAreNoEvents(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	ep.Flush()
	ep.waitUntilInactive()

	assert.Equal(t, 0, len(es.events))
}

func TestEventProcessorStopsSendingEventsAfterUnrecoverableError(t *testing.T) {
	ep, es := createEventProcessorAndSender(epDefaultConfig)
	defer ep.Close()

	es.result = EventSenderResult{MustShutDown: true}

	ie := defaultEventFactory.NewIdentifyEvent(epDefaultUser)
	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	assert.Equal(t, 1, len(es.events))

	ep.SendEvent(ie)
	ep.Flush()
	ep.waitUntilInactive()

	assert.Equal(t, 1, len(es.events)) // no additional payload was sent
}

func TestDiagnosticInitEventIsSent(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	startTime := time.Now()
	diagnosticsManager := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), startTime, nil)
	config := epDefaultConfig
	config.DiagnosticsManager = diagnosticsManager

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()
	ep.waitUntilInactive()

	if assert.Equal(t, 1, len(es.diagnosticEvents)) {
		event := es.diagnosticEvents[0]
		assert.Equal(t, "diagnostic-init", event.GetByKey("kind").StringValue())
		assert.Equal(t, float64(ldtime.UnixMillisFromTime(startTime)), event.GetByKey("creationDate").Float64Value())
	}
}

func TestDiagnosticPeriodicEventsAreSent(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	startTime := time.Now()
	diagnosticsManager := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), startTime, nil)
	config := epDefaultConfig
	config.DiagnosticsManager = diagnosticsManager
	config.forceDiagnosticRecordingInterval = 100 * time.Millisecond

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	// We use a channel for this because we can't predict exactly when the events will be sent
	initEvent := <-es.diagnosticEventsCh
	assert.Equal(t, "diagnostic-init", initEvent.GetByKey("kind").StringValue())
	time0 := uint64(initEvent.GetByKey("creationDate").Float64Value())

	event1 := <-es.diagnosticEventsCh
	assert.Equal(t, "diagnostic", event1.GetByKey("kind").StringValue())
	time1 := uint64(event1.GetByKey("creationDate").Float64Value())
	assert.True(t, time1-time0 >= 70, "event times should follow configured interval: %d, %d", time0, time1)

	event2 := <-es.diagnosticEventsCh
	assert.Equal(t, "diagnostic", event2.GetByKey("kind").StringValue())
	time2 := uint64(event2.GetByKey("creationDate").Float64Value())
	assert.True(t, time2-time1 >= 70, "event times should follow configured interval: %d, %d", time1, time2)
}

func TestDiagnosticPeriodicEventHasEventCounters(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	config := epDefaultConfig
	config.Capacity = 3
	config.forceDiagnosticRecordingInterval = 100 * time.Millisecond
	periodicEventGate := make(chan struct{})

	diagnosticsManager := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), periodicEventGate)
	config.DiagnosticsManager = diagnosticsManager

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	initEvent := <-es.diagnosticEventsCh
	assert.Equal(t, "diagnostic-init", initEvent.GetByKey("kind").StringValue())

	ep.SendEvent(defaultEventFactory.NewCustomEvent("key", lduser.NewUser("userkey"), ldvalue.Null(), false, 0))
	ep.SendEvent(defaultEventFactory.NewCustomEvent("key", lduser.NewUser("userkey"), ldvalue.Null(), false, 0))
	ep.SendEvent(defaultEventFactory.NewCustomEvent("key", lduser.NewUser("userkey"), ldvalue.Null(), false, 0))
	ep.Flush()

	periodicEventGate <- struct{}{} // periodic event won't be sent until we do this

	event1 := <-es.diagnosticEventsCh
	assert.Equal(t, "diagnostic", event1.GetByKey("kind").StringValue())
	assert.Equal(t, 3, event1.GetByKey("eventsInLastBatch").IntValue()) // 1 index, 2 custom
	assert.Equal(t, 1, event1.GetByKey("droppedEvents").IntValue())     // 3rd custom event was dropped
	assert.Equal(t, 2, event1.GetByKey("deduplicatedUsers").IntValue())

	periodicEventGate <- struct{}{}

	event2 := <-es.diagnosticEventsCh // next periodic event - all counters should have been reset
	assert.Equal(t, "diagnostic", event2.GetByKey("kind").StringValue())
	assert.Equal(t, 0, event2.GetByKey("eventsInLastBatch").IntValue())
	assert.Equal(t, 0, event2.GetByKey("droppedEvents").IntValue())
	assert.Equal(t, 0, event2.GetByKey("deduplicatedUsers").IntValue())
}

func jsonEncoding(o interface{}) ldvalue.Value {
	bytes, _ := json.Marshal(o)
	var result ldvalue.Value
	json.Unmarshal(bytes, &result)
	return result
}

func userJsonEncoding(u lduser.User) ldvalue.Value {
	filter := newUserFilter(epDefaultConfig)
	fu := filter.scrubUser(u).filteredUser
	return jsonEncoding(fu)
}

func expectedIdentifyEvent(sourceEvent Event, encodedUser ldvalue.Value) ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("kind", ldvalue.String("identify")).
		Set("key", ldvalue.String(sourceEvent.GetBase().User.GetKey())).
		Set("creationDate", ldvalue.Float64(float64(sourceEvent.GetBase().CreationDate))).
		Set("user", encodedUser).
		Build()
}

func expectedIndexEvent(sourceEvent Event, encodedUser ldvalue.Value) ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("kind", ldvalue.String("index")).
		Set("creationDate", ldvalue.Float64(float64(sourceEvent.GetBase().CreationDate))).
		Set("user", encodedUser).
		Build()
}

func expectedFeatureEvent(sourceEvent FeatureRequestEvent, flag ldeval.FeatureFlag,
	value ldvalue.Value, debug bool, inlineUser *ldvalue.Value) ldvalue.Value {
	kind := "feature"
	if debug {
		kind = "debug"
	}
	expected := ldvalue.ObjectBuild().
		Set("kind", ldvalue.String(kind)).
		Set("key", ldvalue.String(flag.Key)).
		Set("creationDate", ldvalue.Float64(float64(sourceEvent.GetBase().CreationDate))).
		Set("version", ldvalue.Int(flag.Version)).
		Set("value", value).
		Set("default", ldvalue.Null())
	if sourceEvent.Variation != NoVariation {
		expected.Set("variation", ldvalue.Int(sourceEvent.Variation))
	}
	if sourceEvent.Reason.GetKind() != "" {
		expected.Set("reason", jsonEncoding(sourceEvent.Reason))
	} else {
		expected.Set("reason", ldvalue.Null())
	}
	if inlineUser == nil {
		expected.Set("userKey", ldvalue.String(sourceEvent.User.GetKey()))
	} else {
		expected.Set("user", *inlineUser)
	}
	return expected.Build()
}

func assertSummaryEventHasFlag(t *testing.T, flag ldeval.FeatureFlag, output ldvalue.Value) bool {
	if assert.Equal(t, "summary", output.GetByKey("kind").StringValue()) {
		flags := output.GetByKey("features")
		return !flags.GetByKey(flag.Key).IsNull()
	}
	return false
}

func assertSummaryEventHasCounter(t *testing.T, flag ldeval.FeatureFlag, variation int, value ldvalue.Value, count int, output ldvalue.Value) {
	if assertSummaryEventHasFlag(t, flag, output) {
		f := output.GetByKey("features").GetByKey(flag.Key)
		assert.Equal(t, ldvalue.ObjectType, f.Type())
		expected := ldvalue.ObjectBuild().Set("value", value).Set("count", ldvalue.Int(count)).Set("version", ldvalue.Int(flag.Version))
		if variation >= 0 {
			expected.Set("variation", ldvalue.Int(variation))
		}
		counters := []ldvalue.Value{}
		f.GetByKey("counters").Enumerate(func(i int, k string, v ldvalue.Value) bool {
			counters = append(counters, v)
			return true
		})
		assert.Contains(t, counters, expected.Build())
	}
}

// used only for testing - ensures that all pending messages and flushes have completed
func (ep *defaultEventProcessor) waitUntilInactive() {
	m := syncEventsMessage{replyCh: make(chan struct{})}
	ep.inboxCh <- m
	<-m.replyCh // Now we know that all events prior to this call have been processed
}

type mockEventSender struct {
	events             []ldvalue.Value
	diagnosticEvents   []ldvalue.Value
	eventsCh           chan ldvalue.Value
	diagnosticEventsCh chan ldvalue.Value
	result             EventSenderResult
	lock               sync.Mutex
}

func newMockEventSender() *mockEventSender {
	return &mockEventSender{
		eventsCh:           make(chan ldvalue.Value, 100),
		diagnosticEventsCh: make(chan ldvalue.Value, 100),
		result:             EventSenderResult{Success: true},
	}
}

func (ms *mockEventSender) SendEventData(kind EventDataKind, data []byte, eventCount int) EventSenderResult {
	var jsonData ldvalue.Value
	err := json.Unmarshal(data, &jsonData)
	if err != nil {
		panic(err)
	}
	ms.lock.Lock()
	defer ms.lock.Unlock()
	if kind == DiagnosticEventDataKind {
		ms.diagnosticEvents = append(ms.diagnosticEvents, jsonData)
		ms.diagnosticEventsCh <- jsonData
	} else {
		jsonData.Enumerate(func(i int, k string, v ldvalue.Value) bool {
			ms.events = append(ms.events, v)
			ms.eventsCh <- v
			return true
		})
	}
	return ms.result
}

func createEventProcessorAndSender(config EventsConfiguration) (*defaultEventProcessor, *mockEventSender) {
	sender := newMockEventSender()
	config.EventSender = sender
	ep := NewDefaultEventProcessor(config)
	return ep.(*defaultEventProcessor), sender
}

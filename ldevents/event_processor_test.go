package ldevents

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-test-helpers/v3/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v3/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note about the structure of these tests:
//
// 1. It's desirable to keep each test as specific as possible, so that we're not making assertions
// about many details that are extraneous to the main subject of that test, as long as those details
// are more specifically covered by another test. So, for instance, tests that are about feature events
// or custom events are expected to also generate an index event as a side effect, but we should just
// assert that there is one, rather than checking every property of the index event - since we have
// TestIndexEventProperties for that purpose. That way, if there is a bug causing an index event
// property to be wrong, it will show up clearly in that test, rather than causing many failures
// all over the place.
//
// 2. For any tests where the full context JSON will appear in an event, we should use the
// withAndWithoutPrivateAttrs helper to run the test twice, first with a default configuration and
// then with an "all attributes private" configuration. This just verifies that it really is using
// the eventOutputFormatter and eventContextFormatter with the designated configuration when it
// serializes a context. More specific details of private attribute behavior are covered in the tests for
// eventOutputFormatter and eventContextFormatter.
//
// 3. It's preferable to use the matchers and combinators from the matchers package rather than
// the assert and require packages whenever there is (a) an assertion involving JSON values or (b)
// a set of related assertions like "property X equals ___, property Y equals ___" because they
// provide better failure output.

func withAndWithoutPrivateAttrs(t *testing.T, action func(*testing.T, EventsConfiguration)) {
	t.Run("without private attributes", func(t *testing.T) {
		action(t, basicConfigWithoutPrivateAttrs())
	})

	t.Run("with private attributes", func(t *testing.T) {
		config := basicConfigWithoutPrivateAttrs()
		config.AllAttributesPrivate = true
		action(t, config)
	})
}

func withFeatureEventOrCustomEvent(
	t *testing.T,
	action func(
		t *testing.T,
		sendEventFn func(EventProcessor, EventInputContext) (anyEventInput, ldtime.UnixMillisecondTime, []m.Matcher),
		finalEventMatchers []m.Matcher),
) {
	t.Run("from feature event", func(t *testing.T) {
		flag := FlagEventProperties{Key: "flagkey", Version: 11}
		action(t,
			func(ep EventProcessor, context EventInputContext) (anyEventInput, ldtime.UnixMillisecondTime, []m.Matcher) {
				fe := defaultEventFactory.NewEvaluationData(
					flag, context, testEvalDetailWithoutReason, false, ldvalue.Null(), "",
					ldvalue.OptionalInt{}, false,
				)
				ep.RecordEvaluation(fe)
				return fe, fe.CreationDate, nil
			},
			[]m.Matcher{anySummaryEvent()})
	})

	t.Run("from custom event", func(t *testing.T) {
		action(t,
			func(ep EventProcessor, context EventInputContext) (anyEventInput, ldtime.UnixMillisecondTime, []m.Matcher) {
				ce := defaultEventFactory.NewCustomEventData(
					"eventkey", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{},
				)
				ep.RecordCustomEvent(ce)
				return ce, ce.CreationDate, []m.Matcher{anyCustomEvent()}
			},
			nil)
	})
}

func TestIdentifyEventProperties(t *testing.T) {
	withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
		ep, es := createEventProcessorAndSender(config)
		defer ep.Close()

		context := basicContext()
		ie := defaultEventFactory.NewIdentifyEventData(context, ldvalue.OptionalInt{})
		ep.RecordIdentifyEvent(ie)
		ep.Flush()

		assertEventsReceived(t, es, m.JSONEqual(map[string]interface{}{
			"kind":         "identify",
			"creationDate": ie.CreationDate,
			"context":      contextJSON(context, config),
		}))
		es.assertNoMoreEvents(t)
	})
}

func TestMigrationOpEventProperties(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()
	config.forceSampling = true

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	now := ldtime.UnixMillisNow()

	context := basicContext()
	event := MigrationOpEventData{
		BaseEvent: BaseEvent{
			CreationDate: now,
			Context:      context,
		},
		Version:          ldvalue.NewOptionalInt(3),
		FlagKey:          "flag-key",
		Default:          ldmigration.Off,
		Op:               ldmigration.Write,
		Evaluation:       ldreason.NewEvaluationDetail(ldvalue.Bool(true), 0, ldreason.NewEvalReasonFallthrough()),
		SamplingRatio:    ldvalue.NewOptionalInt(100),
		ConsistencyCheck: ldmigration.NewConsistencyCheck(true, 10),
		Error:            map[ldmigration.Origin]struct{}{ldmigration.Old: {}},
		Invoked:          map[ldmigration.Origin]struct{}{ldmigration.Old: {}},
		Latency:          map[ldmigration.Origin]int{ldmigration.Old: 300, ldmigration.New: 400},
	}
	ep.RecordMigrationOpEvent(event)
	ep.Flush()

	assertEventsReceived(t, es, m.JSONEqual(map[string]interface{}{
		"kind":          "migration_op",
		"operation":     "write",
		"creationDate":  now,
		"samplingRatio": 100,
		"contextKeys":   expectedContextKeys(context.context),
		"evaluation": map[string]interface{}{
			"key":       "flag-key",
			"value":     ldvalue.Bool(true),
			"version":   3,
			"variation": 0,
			"reason":    ldreason.NewEvalReasonFallthrough(),
			"default":   "off",
		},
		"measurements": []interface{}{
			map[string]interface{}{
				"key": "invoked",
				"values": map[string]interface{}{
					"old": true,
				},
			},
			map[string]interface{}{
				"key":           "consistent",
				"samplingRatio": 10,
				"value":         true,
			},
			map[string]interface{}{
				"key": "latency_ms",
				"values": map[string]interface{}{
					"old": 300,
					"new": 400,
				},
			},
			map[string]interface{}{
				"key": "error",
				"values": map[string]interface{}{
					"old": true,
				},
			},
		},
	}))
	es.assertNoMoreEvents(t)
}

func TestMigrationOpEventPropertiesWithoutOptionalMeasurements(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()
	config.forceSampling = true

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	now := ldtime.UnixMillisNow()

	context := basicContext()
	event := MigrationOpEventData{
		BaseEvent: BaseEvent{
			CreationDate: now,
			Context:      context,
		},
		FlagKey:       "flag-key",
		Default:       ldmigration.Off,
		Op:            ldmigration.Write,
		Evaluation:    ldreason.NewEvaluationDetail(ldvalue.Bool(true), 0, ldreason.NewEvalReasonFallthrough()),
		SamplingRatio: ldvalue.NewOptionalInt(100),
		Invoked:       map[ldmigration.Origin]struct{}{ldmigration.Old: {}, ldmigration.New: {}},
	}
	ep.RecordMigrationOpEvent(event)
	ep.Flush()

	assertEventsReceived(t, es, m.JSONEqual(map[string]interface{}{
		"kind":          "migration_op",
		"operation":     "write",
		"creationDate":  now,
		"samplingRatio": 100,
		"contextKeys":   expectedContextKeys(context.context),
		"evaluation": map[string]interface{}{
			"key":       "flag-key",
			"value":     ldvalue.Bool(true),
			"variation": 0,
			"reason":    ldreason.NewEvalReasonFallthrough(),
			"default":   "off",
		},
		"measurements": []interface{}{
			map[string]interface{}{
				"key": "invoked",
				"values": map[string]interface{}{
					"old": true,
					"new": true,
				},
			},
		},
	}))
	es.assertNoMoreEvents(t)
}

func TestEventsCanBeDisabledThroughSamplingRatios(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	now := ldtime.UnixMillisNow()

	context := basicContext()
	event := MigrationOpEventData{
		BaseEvent: BaseEvent{
			CreationDate: now,
			Context:      context,
		},
		FlagKey:          "flag-key",
		Default:          ldmigration.Off,
		Op:               ldmigration.Write,
		Evaluation:       ldreason.NewEvaluationDetail(ldvalue.Bool(true), 0, ldreason.NewEvalReasonFallthrough()),
		SamplingRatio:    ldvalue.NewOptionalInt(0),
		ConsistencyCheck: ldmigration.NewConsistencyCheck(true, 10),
		Error:            map[ldmigration.Origin]struct{}{ldmigration.Old: {}},
		Invoked:          map[ldmigration.Origin]struct{}{ldmigration.Old: {}},
		Latency:          map[ldmigration.Origin]int{ldmigration.Old: 300, ldmigration.New: 400},
	}
	ep.RecordMigrationOpEvent(event)

	data := ldvalue.ObjectBuild().SetString("thing", "stuff").Build()
	ce := defaultEventFactory.NewCustomEventData("eventkey", context, data, false, 0, ldvalue.NewOptionalInt(0))
	ep.RecordCustomEvent(ce)

	flag := FlagEventProperties{Key: "flagkey", Version: 11, DebugEventsUntilDate: ldtime.UnixMillisNow() + 1000000}
	fe := defaultEventFactory.NewEvaluationData(flag, context, testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.NewOptionalInt(0), false)
	ep.RecordEvaluation(fe)

	ep.Flush()

	es.assertNoMoreEvents(t)
}

func TestEventsCanForceSampling(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	now := ldtime.UnixMillisNow()
	base := BaseEvent{
		CreationDate: now,
		Context:      basicContext(),
	}

	migrationEvent := MigrationOpEventData{
		BaseEvent:        base,
		FlagKey:          "flag-key",
		Default:          ldmigration.Off,
		Op:               ldmigration.Write,
		Evaluation:       ldreason.NewEvaluationDetail(ldvalue.Bool(true), 0, ldreason.NewEvalReasonFallthrough()),
		SamplingRatio:    ldvalue.NewOptionalInt(0),
		ForceSampling:    true,
		ConsistencyCheck: ldmigration.NewConsistencyCheck(true, 10),
		Error:            map[ldmigration.Origin]struct{}{ldmigration.Old: {}},
		Invoked:          map[ldmigration.Origin]struct{}{ldmigration.Old: {}},
		Latency:          map[ldmigration.Origin]int{ldmigration.Old: 300, ldmigration.New: 400},
	}
	ep.RecordMigrationOpEvent(migrationEvent)
	ep.Flush()

	assertEventsReceived(
		t,
		es,
		m.AllOf(
			m.JSONOptProperty("kind").Should(m.Equal("migration_op")),
			m.JSONOptProperty("samplingRatio").Should(m.Equal(0)),
		),
	)

	featureEvent := EvaluationData{
		BaseEvent:     base,
		Key:           "flag-key",
		Variation:     ldvalue.OptionalInt{},
		Value:         ldvalue.String("on"),
		Default:       ldvalue.String("off"),
		Version:       ldvalue.NewOptionalInt(1),
		Reason:        ldreason.EvaluationReason{},
		SamplingRatio: ldvalue.NewOptionalInt(0),
		ForceSampling: true,
	}
	ep.RecordEvaluation(featureEvent)
	ep.Flush()

	assertEventsReceived(
		t,
		es,
		m.JSONOptProperty("kind").Should(m.Equal("index")),
		m.JSONOptProperty("kind").Should(m.Equal("summary")),
	)

	identityEvent := IdentifyEventData{
		BaseEvent:     base,
		SamplingRatio: ldvalue.NewOptionalInt(0),
		ForceSampling: true,
	}
	ep.RecordIdentifyEvent(identityEvent)
	ep.Flush()

	assertEventsReceived(t, es, m.JSONOptProperty("kind").Should(m.Equal("identify")))

	customEvent := CustomEventData{
		BaseEvent:     base,
		Key:           "custom",
		Data:          ldvalue.String("hi"),
		HasMetric:     false,
		SamplingRatio: ldvalue.NewOptionalInt(0),
		ForceSampling: true,
	}
	ep.RecordCustomEvent(customEvent)
	ep.Flush()

	assertEventsReceived(t, es, m.JSONOptProperty("kind").Should(m.Equal("custom")))
}

func TestFeatureEventIsSummarizedAndNotTrackedByDefault(t *testing.T) {
	withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
		ep, es := createEventProcessorAndSender(config)
		defer ep.Close()

		flag := FlagEventProperties{Key: "flagkey", Version: 11}
		fe := defaultEventFactory.NewEvaluationData(flag, basicContext(), testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
		ep.RecordEvaluation(fe)
		ep.Flush()

		assertEventsReceived(t, es,
			anyIndexEvent(),
			summaryEventWithFlag(flag, summaryCounterPropsFromEval(testEvalDetailWithoutReason, 1)),
		)
		es.assertNoMoreEvents(t)
	})
}

func TestFeatureEventCanBeExcludeFromSummaries(t *testing.T) {
	withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
		ep, es := createEventProcessorAndSender(config)
		defer ep.Close()

		flag := FlagEventProperties{Key: "flagkey", Version: 11}
		fe := defaultEventFactory.NewEvaluationData(flag, basicContext(), testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, true)
		ep.RecordEvaluation(fe)
		ep.Flush()

		assertEventsReceived(t, es,
			anyIndexEvent(),
		)
		es.assertNoMoreEvents(t)
	})
}

func TestIndividualFeatureEventIsQueuedWhenTrackEventsIsTrue(t *testing.T) {
	withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
		ep, es := createEventProcessorAndSender(config)
		defer ep.Close()

		context := basicContext()
		flag := FlagEventProperties{Key: "flagkey", Version: 11, RequireFullEvent: true}
		fe := defaultEventFactory.NewEvaluationData(flag, context, testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
		ep.RecordEvaluation(fe)
		ep.Flush()

		assertEventsReceived(t, es,
			anyIndexEvent(),
			featureEventWithAllProperties(fe, flag, contextJSON(context, config)),
			// Here we also check that the summary count is still the same regardless of TrackEvents
			summaryEventWithFlag(flag,
				summaryCounterPropsFromEval(testEvalDetailWithoutReason, 1)),
		)
		es.assertNoMoreEvents(t)
	})
}

func TestIndividualFeatureEventHasContextAttributesRedactedIfAnonymous(t *testing.T) {
	withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
		ep, es := createEventProcessorAndSender(config)
		defer ep.Close()

		context := ldcontext.NewBuilder(testContextKey).Anonymous(true).SetString("name", "Example name").SetInt("age", 42).Build()
		eventContext := Context(context)
		flag := FlagEventProperties{Key: "flagkey", Version: 11, RequireFullEvent: true}
		fe := defaultEventFactory.NewEvaluationData(flag, eventContext, testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
		ep.RecordEvaluation(fe)
		ep.Flush()

		assertEventsReceived(t, es,
			anyIndexEvent(),
			featureEventWithAllProperties(fe, flag, contextJSON(eventContext, EventsConfiguration{AllAttributesPrivate: true})),
			// Here we also check that the summary count is still the same regardless of TrackEvents
			summaryEventWithFlag(flag,
				summaryCounterPropsFromEval(testEvalDetailWithoutReason, 1)),
		)
		es.assertNoMoreEvents(t)
	})
}

func TestIndexEventProperties(t *testing.T) {
	withFeatureEventOrCustomEvent(t,
		func(t *testing.T, sendEventFn func(EventProcessor, EventInputContext) (anyEventInput, ldtime.UnixMillisecondTime, []m.Matcher), finalEventMatchers []m.Matcher) {
			withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
				ep, es := createEventProcessorAndSender(config)
				defer ep.Close()

				context := basicContext()

				_, creationDate, allEventMatchers := sendEventFn(ep, context)
				ep.Flush()

				allEventMatchers = append(allEventMatchers,
					m.JSONEqual(map[string]interface{}{
						"kind":         "index",
						"creationDate": creationDate,
						"context":      contextJSON(context, config),
					}))
				allEventMatchers = append(allEventMatchers, finalEventMatchers...)
				assertEventsReceived(t, es, allEventMatchers...)
				es.assertNoMoreEvents(t)
			})
		})
}

func TestIndexEventContextKeysAreDeduplicatedForSameKind(t *testing.T) {
	withFeatureEventOrCustomEvent(t,
		func(t *testing.T, sendEventFn func(EventProcessor, EventInputContext) (anyEventInput, ldtime.UnixMillisecondTime, []m.Matcher), finalEventMatchers []m.Matcher) {
			withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
				ep, es := createEventProcessorAndSender(config)
				defer ep.Close()

				context := Context(ldcontext.New("my-key"))

				_, creationDate, allEventMatchers := sendEventFn(ep, context)
				_, _, moreMatchers := sendEventFn(ep, context)
				allEventMatchers = append(allEventMatchers, moreMatchers...)
				ep.Flush()

				allEventMatchers = append(allEventMatchers,
					m.JSONEqual(map[string]interface{}{
						"kind":         "index",
						"creationDate": creationDate,
						"context":      contextJSON(context, config),
					}))
				allEventMatchers = append(allEventMatchers, finalEventMatchers...)
				assertEventsReceived(t, es, allEventMatchers...)
				es.assertNoMoreEvents(t)
			})
		})
}

func TestIndexEventContextKeysAreDeduplicatedSeparatelyForDifferentKinds(t *testing.T) {
	withFeatureEventOrCustomEvent(t,
		func(t *testing.T, sendEventFn func(EventProcessor, EventInputContext) (anyEventInput, ldtime.UnixMillisecondTime, []m.Matcher), finalEventMatchers []m.Matcher) {
			withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
				ep, es := createEventProcessorAndSender(config)
				defer ep.Close()

				key := "my-key"
				context1 := Context(ldcontext.New(key))
				context2 := Context(ldcontext.NewWithKind("org", key))
				context3 := Context(ldcontext.NewMulti(ldcontext.New(key), ldcontext.NewWithKind("other", key)))

				_, creationDate1, allEventMatchers := sendEventFn(ep, context1)
				_, creationDate2, moreMatchers := sendEventFn(ep, context2)
				allEventMatchers = append(allEventMatchers, moreMatchers...)
				_, creationDate3, moreMatchers := sendEventFn(ep, context3)
				allEventMatchers = append(allEventMatchers, moreMatchers...)
				ep.Flush()

				allEventMatchers = append(allEventMatchers,
					m.JSONEqual(map[string]interface{}{
						"kind":         "index",
						"creationDate": creationDate1,
						"context":      contextJSON(context1, config),
					}),
					m.JSONEqual(map[string]interface{}{
						"kind":         "index",
						"creationDate": creationDate2,
						"context":      contextJSON(context2, config),
					}),
					m.JSONEqual(map[string]interface{}{
						"kind":         "index",
						"creationDate": creationDate3,
						"context":      contextJSON(context3, config),
					}))
				allEventMatchers = append(allEventMatchers, finalEventMatchers...)
				assertEventsReceived(t, es, allEventMatchers...)
				es.assertNoMoreEvents(t)
			})
		})
}

func TestDebugEventProperties(t *testing.T) {
	withAndWithoutPrivateAttrs(t, func(t *testing.T, config EventsConfiguration) {
		config.forceSampling = true
		ep, es := createEventProcessorAndSender(config)
		defer ep.Close()

		context := basicContext()
		flag := FlagEventProperties{Key: "flagkey", Version: 11, DebugEventsUntilDate: ldtime.UnixMillisNow() + 1000000}
		fe := defaultEventFactory.NewEvaluationData(flag, context, testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.NewOptionalInt(2), false)
		ep.RecordEvaluation(fe)
		ep.Flush()

		assertEventsReceived(t, es,
			anyIndexEvent(),
			debugEventWithAllProperties(fe, flag, contextJSON(context, config)),
			anySummaryEvent(),
		)
		es.assertNoMoreEvents(t)
	})
}

func TestFeatureEventCanContainReason(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	context := basicContext()
	flag := FlagEventProperties{Key: "flagkey", Version: 11, RequireFullEvent: true}
	fe := defaultEventFactory.NewEvaluationData(flag, context, testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	fe.Reason = ldreason.NewEvalReasonFallthrough()
	ep.RecordEvaluation(fe)
	ep.Flush()

	assertEventsReceived(t, es,
		anyIndexEvent(),
		featureEventWithAllProperties(fe, flag, contextJSON(context, config)),
		anySummaryEvent(),
	)
	es.assertNoMoreEvents(t)
}

func TestDebugEventIsAddedIfFlagIsTemporarilyInDebugMode(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := basicConfigWithoutPrivateAttrs()
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	context := basicContext()
	futureTime := fakeTimeNow + 100
	flag := FlagEventProperties{Key: "flagkey", Version: 11, DebugEventsUntilDate: futureTime}
	fe := eventFactory.NewEvaluationData(flag, context, testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	ep.RecordEvaluation(fe)
	ep.Flush()

	assertEventsReceived(t, es,
		anyIndexEvent(),
		debugEventWithAllProperties(fe, flag, contextJSON(context, config)),
		summaryEventWithFlag(flag, summaryCounterPropsFromEval(testEvalDetailWithoutReason, 1)),
	)
	es.assertNoMoreEvents(t)
}

func TestEventCanBeBothTrackedAndDebugged(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := basicConfigWithoutPrivateAttrs()
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	context := basicContext()
	futureTime := fakeTimeNow + 100
	flag := FlagEventProperties{Key: "flagkey", Version: 11, RequireFullEvent: true, DebugEventsUntilDate: futureTime}
	fe := eventFactory.NewEvaluationData(flag, context, testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	ep.RecordEvaluation(fe)
	ep.Flush()

	assertEventsReceived(t, es,
		anyIndexEvent(),
		featureEventWithAllProperties(fe, flag, contextJSON(context, config)),
		debugEventWithAllProperties(fe, flag, contextJSON(context, config)),
		summaryEventWithFlag(flag, summaryCounterPropsFromEval(testEvalDetailWithoutReason, 1)),
	)
	es.assertNoMoreEvents(t)
}

func TestDebugModeExpiresBasedOnClientTimeIfClientTimeIsLater(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := basicConfigWithoutPrivateAttrs()
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	// Pick a server time that is somewhat behind the client time
	serverTime := fakeTimeNow - 20000
	es.result = EventSenderResult{Success: true, TimeFromServer: serverTime}

	// Send and flush an event we don't care about, just to set the last server time
	ie := eventFactory.NewIdentifyEventData(basicContext(), ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(ie)
	ep.Flush()
	assertEventsReceived(t, es, anyIdentifyEvent())

	// Now send an event with debug mode on, with a "debug until" time that is further in
	// the future than the server time, but in the past compared to the client.
	debugUntil := serverTime + 1000
	flag := FlagEventProperties{Key: "flagkey", Version: 11, DebugEventsUntilDate: debugUntil}
	fe := eventFactory.NewEvaluationData(flag, basicContext(), testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	ep.RecordEvaluation(fe)
	ep.Flush()

	// should get a summary event only, not a debug event
	assertEventsReceived(t, es, anySummaryEvent())
	es.assertNoMoreEvents(t)
}

func TestDebugModeExpiresBasedOnServerTimeIfServerTimeIsLater(t *testing.T) {
	fakeTimeNow := ldtime.UnixMillisecondTime(1000000)
	config := basicConfigWithoutPrivateAttrs()
	config.currentTimeProvider = func() ldtime.UnixMillisecondTime { return fakeTimeNow }
	eventFactory := NewEventFactory(false, config.currentTimeProvider)

	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	// Pick a server time that is somewhat ahead of the client time
	serverTime := fakeTimeNow + 20000
	es.result = EventSenderResult{Success: true, TimeFromServer: serverTime}

	// Send and flush an event we don't care about, just to set the last server time
	ie := eventFactory.NewIdentifyEventData(basicContext(), ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(ie)
	ep.Flush()
	assertEventsReceived(t, es, anyIdentifyEvent())

	// Now send an event with debug mode on, with a "debug until" time that is further in
	// the future than the client time, but in the past compared to the server.
	debugUntil := serverTime - 1000
	flag := FlagEventProperties{Key: "flagkey", Version: 11, DebugEventsUntilDate: debugUntil}
	fe := eventFactory.NewEvaluationData(flag, basicContext(), testEvalDetailWithoutReason, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	ep.RecordEvaluation(fe)
	ep.Flush()

	// should get a summary event only, not a debug event
	assertEventsReceived(t, es, anySummaryEvent())
	es.assertNoMoreEvents(t)
}

func TestNonTrackedEventsAreSummarized(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	context := basicContext()
	flag1 := FlagEventProperties{Key: "flagkey1", Version: 11}
	flag2 := FlagEventProperties{Key: "flagkey2", Version: 22}
	flag1Eval := ldreason.NewEvaluationDetail(ldvalue.String("value1"), 2, noReason)
	flag2Eval := ldreason.NewEvaluationDetail(ldvalue.String("value2"), 3, noReason)
	fe1 := defaultEventFactory.NewEvaluationData(flag1, context, flag1Eval, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	fe2 := defaultEventFactory.NewEvaluationData(flag2, context, flag2Eval, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	fe3 := defaultEventFactory.NewEvaluationData(flag2, context, flag2Eval, false, ldvalue.Null(), "", ldvalue.OptionalInt{}, false)
	ep.RecordEvaluation(fe1)
	ep.RecordEvaluation(fe2)
	ep.RecordEvaluation(fe3)
	ep.Flush()

	assertEventsReceived(t, es, anyIndexEvent())

	assertEventsReceived(t, es, m.AllOf(
		m.JSONProperty("startDate").Should(equalNumericTime(fe1.CreationDate)),
		m.JSONProperty("endDate").Should(equalNumericTime(fe3.CreationDate)),
		summaryEventWithFlag(flag1, summaryCounterPropsFromEval(flag1Eval, 1)),
		summaryEventWithFlag(flag2, summaryCounterPropsFromEval(flag2Eval, 2)),
	))

	es.assertNoMoreEvents(t)
}

func TestCustomEventProperties(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()
	config.forceSampling = true
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	context := basicContext()
	data := ldvalue.ObjectBuild().SetString("thing", "stuff").Build()
	ce := defaultEventFactory.NewCustomEventData("eventkey", context, data, false, 0, ldvalue.NewOptionalInt(2))
	ep.RecordCustomEvent(ce)
	ep.Flush()

	customEventMatcher := m.JSONEqual(map[string]interface{}{
		"kind":          "custom",
		"creationDate":  ce.CreationDate,
		"key":           ce.Key,
		"data":          data,
		"samplingRatio": 2,
		"contextKeys":   expectedContextKeys(context.context),
	})
	assertEventsReceived(t, es,
		anyIndexEvent(),
		customEventMatcher,
	)
	es.assertNoMoreEvents(t)
}

func TestCustomEventCanHaveMetricValue(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	context := basicContext()
	data := ldvalue.ObjectBuild().SetString("thing", "stuff").Build()
	metric := float64(2.5)
	ce := defaultEventFactory.NewCustomEventData("eventkey", context, data, true, metric, ldvalue.OptionalInt{})
	ep.RecordCustomEvent(ce)
	ep.Flush()

	customEventMatcher := m.JSONEqual(map[string]interface{}{
		"kind":         "custom",
		"creationDate": ce.CreationDate,
		"key":          ce.Key,
		"data":         data,
		"metricValue":  metric,
		"contextKeys":  expectedContextKeys(context.context),
	})
	assertEventsReceived(t, es,
		anyIndexEvent(),
		customEventMatcher,
	)
	es.assertNoMoreEvents(t)
}

func TestRawEventIsQueued(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	rawData := json.RawMessage(`{"kind":"alias","arbitrary":["we","don't","care","what's","in","here"]}`)
	ep.RecordRawEvent(rawData)
	ep.Flush()
	ep.waitUntilInactive()

	assertEventsReceived(t, es, m.JSONEqual(rawData))
	es.assertNoMoreEvents(t)
}

func TestShutdownPreventsFurtherEventsFromReachingSender(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	es.result = EventSenderResult{MustShutDown: true}
	firstMessage := json.RawMessage(`{"kind":"raw","arbitrary":["first","attempt"]}`)
	ep.RecordRawEvent(firstMessage)
	ep.Flush()
	ep.waitUntilInactive()

	es.result = EventSenderResult{Success: true}
	secondMessage := json.RawMessage(`{"kind":"raw","arbitrary":["second", "attempt"]}`)
	ep.RecordRawEvent(secondMessage)
	ep.RecordRawEvent(secondMessage)
	ep.Flush()

	// We have the first event because the sender is a mock that just records
	// everything sent to it, regardless of its response. However, once we
	// return a shutdown message, the processor should stop sending new events
	// to the sender mock.
	assertEventsReceived(t, es, m.JSONEqual(firstMessage))
	es.assertNoMoreEvents(t)
}

func TestPeriodicFlush(t *testing.T) {
	config := basicConfigWithoutPrivateAttrs()
	config.FlushInterval = 10 * time.Millisecond
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	context := basicContext()
	ie := defaultEventFactory.NewIdentifyEventData(context, ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(ie)

	assertEventsReceived(t, es, identifyEventForContextKey(context.context.Key()))
	es.assertNoMoreEvents(t)
}

func TestBlockingFlush(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	senderGateCh := make(chan struct{}, 1)
	senderWaitingCh := make(chan struct{}, 1)
	es.setGate(senderGateCh, senderWaitingCh)

	didFlush := make(chan struct{}, 1)
	go func() {
		<-senderWaitingCh
		time.Sleep(time.Millisecond * 100)
		didFlush <- struct{}{}
		senderGateCh <- struct{}{}
	}()

	ep.RecordIdentifyEvent(defaultEventFactory.NewIdentifyEventData(basicContext(), ldvalue.OptionalInt{}))
	success := ep.FlushBlocking(time.Second)

	assert.True(t, success)
	assert.NotEqual(t, 0, len(didFlush))
}

func TestBlockingFlushTimeout(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	senderGateCh := make(chan struct{}, 1)
	senderWaitingCh := make(chan struct{}, 1)
	es.setGate(senderGateCh, senderWaitingCh)

	didFlush := make(chan struct{}, 1)
	go func() {
		<-senderWaitingCh
		time.Sleep(time.Millisecond * 500)
		didFlush <- struct{}{}
		senderGateCh <- struct{}{}
	}()

	ep.RecordIdentifyEvent(defaultEventFactory.NewIdentifyEventData(basicContext(), ldvalue.OptionalInt{}))
	success := ep.FlushBlocking(time.Millisecond * 50)

	assert.False(t, success)
	assert.Equal(t, 0, len(didFlush))
}

func TestClosingEventProcessorForcesSynchronousFlush(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	context := basicContext()
	ie := defaultEventFactory.NewIdentifyEventData(context, ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(ie)
	ep.Close()

	assertEventsReceived(t, es, identifyEventForContextKey(context.context.Key()))
	es.assertNoMoreEvents(t)
}

func TestPeriodicUserKeysFlush(t *testing.T) {
	// This test overrides the context key flush interval to a small value and verifies that a new
	// index event is generated for a context after the context keys have been flushed.
	config := basicConfigWithoutPrivateAttrs()
	config.UserKeysFlushInterval = time.Millisecond * 100
	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	context := basicContext()
	event1 := defaultEventFactory.NewCustomEventData("event1", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{})
	event2 := defaultEventFactory.NewCustomEventData("event2", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{})
	ep.RecordCustomEvent(event1)
	ep.RecordCustomEvent(event2)
	ep.Flush()

	// We're relying on the context key flush not happening in between event1 and event2, so we should get
	// a single index event for the context.
	assertEventsReceived(t, es,
		indexEventForContextKey(context.context.Key()),
		customEventWithEventKey("event1"),
		customEventWithEventKey("event2"),
	)

	// Now wait long enough for the context key cache to be flushed
	<-time.After(200 * time.Millisecond)

	// Referencing the same context in a new event should produce a new index event
	event3 := defaultEventFactory.NewCustomEventData("event3", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{})
	ep.RecordCustomEvent(event3)
	ep.Flush()
	assertEventsReceived(t, es,
		indexEventForContextKey(context.context.Key()),
		customEventWithEventKey("event3"),
	)
	es.assertNoMoreEvents(t)
}

func TestNothingIsSentIfThereAreNoEvents(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	ep.FlushBlocking(time.Second)

	es.assertNoMoreEvents(t)
}

func TestEventProcessorStopsSendingEventsAfterUnrecoverableError(t *testing.T) {
	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	es.result = EventSenderResult{MustShutDown: true}

	ie := defaultEventFactory.NewIdentifyEventData(basicContext(), ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(ie)
	ep.Flush()
	es.awaitEvent(t)

	ep.RecordIdentifyEvent(ie)
	ep.FlushBlocking(time.Second)

	es.assertNoMoreEvents(t)
}

func TestDiagnosticInitEventIsSent(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	startTime := time.Now()
	diagnosticsManager := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), startTime, nil)
	config := basicConfigWithoutPrivateAttrs()
	config.DiagnosticsManager = diagnosticsManager

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	event := es.awaitDiagnosticEvent(t)
	m.In(t).Assert(event, m.AllOf(
		eventKindIs("diagnostic-init"),
		m.JSONProperty("creationDate").Should(equalNumericTime(ldtime.UnixMillisFromTime(startTime))),
	))
	es.assertNoMoreDiagnosticEvents(t)
}

func TestDiagnosticPeriodicEventsAreSent(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	startTime := time.Now()
	diagnosticsManager := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), startTime, nil)
	config := basicConfigWithoutPrivateAttrs()
	config.DiagnosticsManager = diagnosticsManager
	config.forceDiagnosticRecordingInterval = 100 * time.Millisecond

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	// We use a channel for this because we can't predict exactly when the events will be sent
	initEvent := es.awaitDiagnosticEvent(t)
	m.In(t).Assert(initEvent, eventKindIs("diagnostic-init"))
	time0 := requireCreationDate(t, initEvent)

	event1 := es.awaitDiagnosticEvent(t)
	m.In(t).Assert(event1, eventKindIs("diagnostic"))
	time1 := requireCreationDate(t, event1)
	assert.True(t, time1-time0 >= 70, "event times should follow configured interval: %d, %d", time0, time1)

	event2 := es.awaitDiagnosticEvent(t)
	m.In(t).Assert(event2, eventKindIs("diagnostic"))
	time2 := requireCreationDate(t, event2)
	assert.True(t, time2-time1 >= 70, "event times should follow configured interval: %d, %d", time1, time2)
}

func TestDiagnosticPeriodicEventHasEventCounters(t *testing.T) {
	id := NewDiagnosticID("sdkkey")
	config := basicConfigWithoutPrivateAttrs()
	config.Capacity = 3
	config.forceDiagnosticRecordingInterval = 100 * time.Millisecond
	periodicEventGate := make(chan struct{})

	diagnosticsManager := NewDiagnosticsManager(id, ldvalue.Null(), ldvalue.Null(), time.Now(), periodicEventGate)
	config.DiagnosticsManager = diagnosticsManager

	ep, es := createEventProcessorAndSender(config)
	defer ep.Close()

	initEvent := es.awaitDiagnosticEvent(t)
	m.In(t).Assert(initEvent, eventKindIs("diagnostic-init"))

	context := Context(lduser.NewUser("userkey"))
	ep.RecordCustomEvent(defaultEventFactory.NewCustomEventData("key", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{}))
	ep.RecordCustomEvent(defaultEventFactory.NewCustomEventData("key", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{}))
	ep.RecordCustomEvent(defaultEventFactory.NewCustomEventData("key", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{}))
	ep.Flush()

	periodicEventGate <- struct{}{} // periodic event won't be sent until we do this

	event1 := es.awaitDiagnosticEvent(t)
	m.In(t).Assert(event1, m.AllOf(
		eventKindIs("diagnostic"),
		m.JSONProperty("eventsInLastBatch").Should(m.Equal(3)), // 1 index, 2 custom
		m.JSONProperty("droppedEvents").Should(m.Equal(1)),     // 3rd custom event was dropped
		m.JSONProperty("deduplicatedUsers").Should(m.Equal(2)),
	))

	periodicEventGate <- struct{}{}

	event2 := es.awaitDiagnosticEvent(t) // next periodic event - all counters should have been reset
	m.In(t).Assert(event2, m.AllOf(
		eventKindIs("diagnostic"),
		m.JSONProperty("eventsInLastBatch").Should(m.Equal(0)),
		m.JSONProperty("droppedEvents").Should(m.Equal(0)),
		m.JSONProperty("deduplicatedUsers").Should(m.Equal(0)),
	))
}

func TestEventsAreKeptInBufferIfAllFlushWorkersAreBusy(t *testing.T) {
	// Note that in the current implementation, although the intention was that we would cancel a flush
	// if there's not an available flush worker, instead what happens is that we will queue *one* flush
	// in that case, and then cancel the *next* flush if the workers are still busy. This is because the
	// flush payload channel has a buffer size of 1, rather than zero. The test below verifies the
	// current behavior.

	user1 := Context(lduser.NewUser("user1"))
	user2 := Context(lduser.NewUser("user2"))
	user3 := Context(lduser.NewUser("user3"))

	ep, es := createEventProcessorAndSender(basicConfigWithoutPrivateAttrs())
	defer ep.Close()

	senderGateCh := make(chan struct{}, maxFlushWorkers)
	senderWaitingCh := make(chan struct{}, maxFlushWorkers)
	es.setGate(senderGateCh, senderWaitingCh)

	arbitraryContext := Context(ldcontext.New("other"))
	for i := 0; i < maxFlushWorkers; i++ {
		ep.RecordIdentifyEvent(defaultEventFactory.NewIdentifyEventData(arbitraryContext, ldvalue.OptionalInt{}))
		ep.Flush()
		_ = es.awaitEvent(t) // we don't need to see this payload, just throw it away
	}

	// Each of the worker goroutines should now be blocked waiting for senderGateCh. We can tell when
	// they have all gotten to that point because they have posted to senderReadyCh.
	for i := 0; i < maxFlushWorkers; i++ {
		<-senderWaitingCh
	}
	es.assertNoMoreEvents(t)
	assert.Equal(t, maxFlushWorkers, es.getPayloadCount())

	// Now, put an event in the buffer and try to flush again. In the current implementation (see
	// above) this payload gets queued in a holding area, and will be flushed after a worker
	// becomes free.
	extraEvent1 := defaultEventFactory.NewIdentifyEventData(user1, ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(extraEvent1)
	ep.Flush()

	// Do an additional flush with another event. This time, the event processor should see that there's
	// no space available and simply ignore the flush request. There's no way to verify programmatically
	// that this has happened, so just give it a short delay.
	extraEvent2 := defaultEventFactory.NewIdentifyEventData(user2, ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(extraEvent2)
	ep.Flush()
	<-time.After(100 * time.Millisecond)
	es.assertNoMoreEvents(t)

	// Enqueue a third event. The current payload should now be extraEvent2 + extraEvent3.
	extraEvent3 := defaultEventFactory.NewIdentifyEventData(user3, ldvalue.OptionalInt{})
	ep.RecordIdentifyEvent(extraEvent3)

	// Now allow the workers to unblock.
	for i := 0; i < maxFlushWorkers; i++ {
		senderGateCh <- struct{}{}
	}

	// The first unblocked worker should pick up the queued payload with event1.
	senderGateCh <- struct{}{}
	assertEventsReceived(t, es, identifyEventForContextKey(user1.context.Key()))

	// Now a flush should succeed and send the current payload.
	senderGateCh <- struct{}{}
	ep.Flush()
	assertEventsReceived(t, es,
		identifyEventForContextKey(user2.context.Key()),
		identifyEventForContextKey(user3.context.Key()),
	)
	assert.Equal(t, maxFlushWorkers+2, es.getPayloadCount())
}

// used only for testing - ensures that all pending messages and flushes have completed
func (ep *defaultEventProcessor) waitUntilInactive() {
	m := syncEventsMessage{replyCh: make(chan struct{})}
	ep.inboxCh <- m
	<-m.replyCh // Now we know that all events prior to this call have been processed
}

func createEventProcessorAndSender(config EventsConfiguration) (*defaultEventProcessor, *mockEventSender) {
	sender := newMockEventSender()
	config.EventSender = sender
	ep := NewDefaultEventProcessor(config)
	return ep.(*defaultEventProcessor), sender
}

func assertEventsReceived(t *testing.T, es *mockEventSender, matchers ...m.Matcher) {
	t.Helper()
	received := make([]json.RawMessage, 0, len(matchers))
	for range matchers {
		if event, ok := es.tryAwaitEvent(); ok {
			received = append(received, event)
		} else {
			require.Fail(t, "timed out waiting for analytics event(s)", "wanted %d event(s); got: %s",
				len(matchers), jsonhelpers.ToJSONString(received))
		}
	}
	// Use the ItemsInAnyOrder matcher because the exact ordering of events is not significant.
	m.In(t).Assert(received, m.ItemsInAnyOrder(matchers...))
}

func requireCreationDate(t *testing.T, eventData json.RawMessage) ldtime.UnixMillisecondTime {
	m.In(t).Require(eventData, m.JSONProperty("creationDate").Should(valueIsPositiveNonZeroInteger()))
	return ldtime.UnixMillisecondTime(ldvalue.Parse(eventData).GetByKey("creationDate").Float64Value())
}

package ldevents

import (
	"encoding/json"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	m "github.com/launchdarkly/go-test-helpers/v3/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	withoutReasons = NewEventFactory(false, fakeTimeFn)
	withReasons    = NewEventFactory(true, fakeTimeFn)
)

func withTestContextsAndConfigs(t *testing.T, action func(*testing.T, EventInputContext, EventsConfiguration)) {
	singleCtx := Context(ldcontext.New("user-key"))
	multiCtx := Context(ldcontext.NewMulti(ldcontext.New("user-key"), ldcontext.NewWithKind("org", "org-key")))

	privateConfig := basicConfigWithoutPrivateAttrs()
	privateConfig.AllAttributesPrivate = true

	t.Run("single kind, no private attributes", func(t *testing.T) {
		action(t, singleCtx, basicConfigWithoutPrivateAttrs())
	})

	t.Run("multi-kind, no private attributes", func(t *testing.T) {
		action(t, multiCtx, basicConfigWithoutPrivateAttrs())
	})

	t.Run("single kind, with private attributes", func(t *testing.T) {
		action(t, singleCtx, privateConfig)
	})

	t.Run("multi-kind, with private attributes", func(t *testing.T) {
		action(t, multiCtx, privateConfig)
	})
}

func TestEventOutputFullEvents(t *testing.T) {
	withTestContextsAndConfigs(t, func(t *testing.T, context EventInputContext, config EventsConfiguration) {
		flag := FlagEventProperties{Key: "flagkey", Version: 100}

		formatter := eventOutputFormatter{
			contextFormatter: newEventContextFormatter(config),
			config:           config,
		}

		// In this test, we are assuming that the output of eventContextFormatter is correct with regard to
		// private attributes; those details are covered in the tests for eventContextFormatter itself. We
		// just want to verify here that eventOutputFormatter is actually *using* eventContextFormatter with
		// the specified configuration.
		contextJSON := contextJSON(context, config)
		contextKeys := expectedContextKeys(context.context)

		t.Run("feature", func(t *testing.T) {
			event1 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.OptionalInt{}, false)
			verifyEventOutput(t, formatter, event1,
				m.JSONEqual(map[string]interface{}{
					"kind":         "feature",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"version":      flag.Version,
					"context":      contextJSON,
					"variation":    1,
					"value":        "v",
					"default":      "dv",
				}))

			event1r := withReasons.NewEvaluationData(flag, context,
				ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, ldreason.NewEvalReasonFallthrough()),
				false, ldvalue.String("dv"), "", ldvalue.OptionalInt{}, false)
			verifyEventOutput(t, formatter, event1r,
				m.JSONEqual(map[string]interface{}{
					"kind":         "feature",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"version":      flag.Version,
					"context":      contextJSON,
					"variation":    1,
					"value":        "v",
					"default":      "dv",
					"reason":       json.RawMessage(`{"kind":"FALLTHROUGH"}`),
				}))

			event2 := withoutReasons.NewEvaluationData(flag, context, ldreason.EvaluationDetail{Value: ldvalue.String("v")},
				false, ldvalue.String("dv"), "", ldvalue.OptionalInt{}, false)
			event2.Variation = ldvalue.OptionalInt{}
			verifyEventOutput(t, formatter, event2,
				m.JSONEqual(map[string]interface{}{
					"kind":         "feature",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"version":      flag.Version,
					"context":      contextJSON,
					"value":        "v",
					"default":      "dv",
				}))

			event3 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "pre", ldvalue.OptionalInt{}, false)
			verifyEventOutput(t, formatter, event3,
				m.JSONEqual(map[string]interface{}{
					"kind":         "feature",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"version":      flag.Version,
					"context":      contextJSON,
					"variation":    1,
					"value":        "v",
					"default":      "dv",
					"prereqOf":     "pre",
				}))

			event4 := withoutReasons.NewUnknownFlagEvaluationData("flagkey", context,
				ldvalue.String("dv"), ldreason.EvaluationReason{})
			verifyEventOutput(t, formatter, event4,
				m.JSONEqual(map[string]interface{}{
					"kind":         "feature",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"context":      contextJSON,
					"value":        "dv",
					"default":      "dv",
				}))

			// A sampling ratio of 1 will not appear in the JSON output
			event5 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.NewOptionalInt(1), false)
			verifyEventOutput(t, formatter, event5,
				m.JSONEqual(map[string]interface{}{
					"kind":         "feature",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"version":      flag.Version,
					"context":      contextJSON,
					"variation":    1,
					"value":        "v",
					"default":      "dv",
				}))

			// We should explicitly include the sampling ratio when specified to something other than 1
			event6 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.NewOptionalInt(10), false)
			verifyEventOutput(t, formatter, event6,
				m.JSONEqual(map[string]interface{}{
					"kind":          "feature",
					"creationDate":  fakeTime,
					"key":           flag.Key,
					"version":       flag.Version,
					"context":       contextJSON,
					"samplingRatio": 10,
					"variation":     1,
					"value":         "v",
					"default":       "dv",
				}))

			// This includes when the sampling ratio is 0
			event7 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.NewOptionalInt(0), false)
			verifyEventOutput(t, formatter, event7,
				m.JSONEqual(map[string]interface{}{
					"kind":          "feature",
					"creationDate":  fakeTime,
					"key":           flag.Key,
					"version":       flag.Version,
					"context":       contextJSON,
					"samplingRatio": 0,
					"variation":     1,
					"value":         "v",
					"default":       "dv",
				}))
		})

		t.Run("debug", func(t *testing.T) {
			event1 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.OptionalInt{}, false)
			event1.debug = true
			verifyEventOutput(t, formatter, event1,
				m.JSONEqual(map[string]interface{}{
					"kind":         "debug",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"version":      flag.Version,
					"context":      contextJSON,
					"variation":    1,
					"value":        "v",
					"default":      "dv",
				}))

			// A sampling ratio of 1 will not appear in the JSON output
			event2 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.NewOptionalInt(1), false)
			event2.debug = true
			verifyEventOutput(t, formatter, event2,
				m.JSONEqual(map[string]interface{}{
					"kind":         "debug",
					"creationDate": fakeTime,
					"key":          flag.Key,
					"version":      flag.Version,
					"context":      contextJSON,
					"variation":    1,
					"value":        "v",
					"default":      "dv",
				}))

			// We should explicitly include the sampling ratio when specified to something other than 1
			event3 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.NewOptionalInt(10), false)
			event3.debug = true
			verifyEventOutput(t, formatter, event3,
				m.JSONEqual(map[string]interface{}{
					"kind":          "debug",
					"creationDate":  fakeTime,
					"key":           flag.Key,
					"version":       flag.Version,
					"context":       contextJSON,
					"samplingRatio": 10,
					"variation":     1,
					"value":         "v",
					"default":       "dv",
				}))

			// This includes when the sampling ratio is 0
			event4 := withoutReasons.NewEvaluationData(flag, context, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
				false, ldvalue.String("dv"), "", ldvalue.NewOptionalInt(0), false)
			event4.debug = true
			verifyEventOutput(t, formatter, event4,
				m.JSONEqual(map[string]interface{}{
					"kind":          "debug",
					"creationDate":  fakeTime,
					"key":           flag.Key,
					"version":       flag.Version,
					"context":       contextJSON,
					"samplingRatio": 0,
					"variation":     1,
					"value":         "v",
					"default":       "dv",
				}))
		})

		t.Run("identify", func(t *testing.T) {
			event1 := withoutReasons.NewIdentifyEventData(context, ldvalue.OptionalInt{})
			verifyEventOutput(t, formatter, event1,
				m.JSONEqual(map[string]interface{}{
					"kind":         "identify",
					"creationDate": fakeTime,
					"context":      contextJSON,
				}))

			// A sampling ratio of 1 will not appear in the JSON output
			event2 := withoutReasons.NewIdentifyEventData(context, ldvalue.NewOptionalInt(1))
			verifyEventOutput(t, formatter, event2,
				m.JSONEqual(map[string]interface{}{
					"kind":         "identify",
					"creationDate": fakeTime,
					"context":      contextJSON,
				}))

			// We should explicitly include the sampling ratio when specified to something other than 1
			event3 := withoutReasons.NewIdentifyEventData(context, ldvalue.NewOptionalInt(10))
			verifyEventOutput(t, formatter, event3,
				m.JSONEqual(map[string]interface{}{
					"kind":          "identify",
					"creationDate":  fakeTime,
					"context":       contextJSON,
					"samplingRatio": 10,
				}))

			// This includes when the sampling ratio is 0
			event4 := withoutReasons.NewIdentifyEventData(context, ldvalue.NewOptionalInt(0))
			verifyEventOutput(t, formatter, event4,
				m.JSONEqual(map[string]interface{}{
					"kind":          "identify",
					"creationDate":  fakeTime,
					"context":       contextJSON,
					"samplingRatio": 0,
				}))
		})

		t.Run("custom", func(t *testing.T) {
			event1 := withoutReasons.NewCustomEventData("eventkey", context, ldvalue.Null(), false, 0, ldvalue.OptionalInt{})
			verifyEventOutput(t, formatter, event1,
				m.JSONEqual(map[string]interface{}{
					"kind":         "custom",
					"key":          "eventkey",
					"creationDate": fakeTime,
					"contextKeys":  contextKeys,
				}))

			event2 := withoutReasons.NewCustomEventData("eventkey", context, ldvalue.String("d"), false, 0, ldvalue.OptionalInt{})
			verifyEventOutput(t, formatter, event2,
				m.JSONEqual(map[string]interface{}{
					"kind":         "custom",
					"key":          "eventkey",
					"creationDate": fakeTime,
					"contextKeys":  contextKeys,
					"data":         "d",
				}))

			event3 := withoutReasons.NewCustomEventData("eventkey", context, ldvalue.Null(), true, 2.5, ldvalue.OptionalInt{})
			verifyEventOutput(t, formatter, event3,
				m.JSONEqual(map[string]interface{}{
					"kind":         "custom",
					"key":          "eventkey",
					"creationDate": fakeTime,
					"contextKeys":  contextKeys,
					"metricValue":  2.5,
				}))

			// A sampling ratio of 1 will not appear in the JSON output
			event4 := withoutReasons.NewCustomEventData("eventkey", context, ldvalue.Null(), false, 0, ldvalue.NewOptionalInt(1))
			verifyEventOutput(t, formatter, event4,
				m.JSONEqual(map[string]interface{}{
					"kind":         "custom",
					"key":          "eventkey",
					"creationDate": fakeTime,
					"contextKeys":  contextKeys,
				}))

			// We should explicitly include the sampling ratio when specified to something other than 1
			event5 := withoutReasons.NewCustomEventData("eventkey", context, ldvalue.Null(), false, 0, ldvalue.NewOptionalInt(10))
			verifyEventOutput(t, formatter, event5,
				m.JSONEqual(map[string]interface{}{
					"kind":          "custom",
					"key":           "eventkey",
					"creationDate":  fakeTime,
					"contextKeys":   contextKeys,
					"samplingRatio": 10,
				}))

			// This includes when the sampling ratio is 0
			event6 := withoutReasons.NewCustomEventData("eventkey", context, ldvalue.Null(), false, 0, ldvalue.NewOptionalInt(0))
			verifyEventOutput(t, formatter, event6,
				m.JSONEqual(map[string]interface{}{
					"kind":          "custom",
					"key":           "eventkey",
					"creationDate":  fakeTime,
					"contextKeys":   contextKeys,
					"samplingRatio": 0,
				}))
		})

		t.Run("index", func(t *testing.T) {
			event1 := indexEvent{BaseEvent: BaseEvent{CreationDate: fakeTime, Context: context}}
			verifyEventOutput(t, formatter, event1,
				m.JSONEqual(map[string]interface{}{
					"kind":         "index",
					"creationDate": fakeTime,
					"context":      contextJSON,
				}))
		})

		t.Run("raw", func(t *testing.T) {
			rawData := json.RawMessage(`{"kind":"alias","arbitrary":["we","don't","care","what's","in","here"]}`)
			event := rawEvent{data: rawData}
			verifyEventOutput(t, formatter, event, m.JSONEqual(rawData))
		})
	})
}

func TestEventOutputSummaryEvents(t *testing.T) {
	user := Context(lduser.NewUser("u"))
	flag1v1 := FlagEventProperties{Key: "flag1", Version: 100}
	flag1v2 := FlagEventProperties{Key: "flag1", Version: 200}
	flag1Default := ldvalue.String("default1")
	flag2 := FlagEventProperties{Key: "flag2", Version: 1}
	flag2Default := ldvalue.String("default2")

	formatter := eventOutputFormatter{
		contextFormatter: newEventContextFormatter(basicConfigWithoutPrivateAttrs()),
		config:           basicConfigWithoutPrivateAttrs(),
	}

	t.Run("summary - single flag, single counter", func(t *testing.T) {
		es1 := newEventSummarizer()
		event1 := withoutReasons.NewEvaluationData(flag1v1, user, ldreason.NewEvaluationDetail(ldvalue.String("v"), 1, noReason),
			false, ldvalue.String("dv"), "", ldvalue.OptionalInt{}, false)
		es1.summarizeEvent(event1)
		verifySummaryEventOutput(t, formatter, es1.snapshot(),
			m.JSONEqual(map[string]interface{}{
				"kind":      "summary",
				"startDate": fakeTime,
				"endDate":   fakeTime,
				"features": map[string]interface{}{
					"flag1": map[string]interface{}{
						"counters":     json.RawMessage(`[{"count":1,"value":"v","variation":1,"version":100}]`),
						"contextKinds": []string{"user"},
						"default":      "dv",
					},
				},
			}))

		es2 := newEventSummarizer()
		event2 := withoutReasons.NewEvaluationData(flag1v1, user, ldreason.EvaluationDetail{Value: ldvalue.String("dv")},
			false, ldvalue.String("dv"), "", ldvalue.OptionalInt{}, false)
		event2.Variation = ldvalue.OptionalInt{}
		es2.summarizeEvent(event2)
		verifySummaryEventOutput(t, formatter, es2.snapshot(),
			m.JSONEqual(map[string]interface{}{
				"kind":      "summary",
				"startDate": fakeTime,
				"endDate":   fakeTime,
				"features": map[string]interface{}{
					"flag1": map[string]interface{}{
						"counters":     json.RawMessage(`[{"count":1,"value":"dv","version":100}]`),
						"contextKinds": []string{"user"},
						"default":      "dv",
					},
				},
			}))

		es3 := newEventSummarizer()
		event3 := withoutReasons.NewUnknownFlagEvaluationData("flagkey", user,
			ldvalue.String("dv"), ldreason.EvaluationReason{})
		es3.summarizeEvent(event3)
		verifySummaryEventOutput(t, formatter, es3.snapshot(),
			m.JSONEqual(map[string]interface{}{
				"kind":      "summary",
				"startDate": fakeTime,
				"endDate":   fakeTime,
				"features": map[string]interface{}{
					"flagkey": map[string]interface{}{
						"counters":     json.RawMessage(`[{"count":1,"value":"dv","unknown":true}]`),
						"contextKinds": []string{"user"},
						"default":      "dv",
					},
				},
			}))
	})

	t.Run("summary - multiple counters", func(t *testing.T) {
		es := newEventSummarizer()
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v1, user, ldreason.NewEvaluationDetail(ldvalue.String("a"), 1, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v1, user, ldreason.NewEvaluationDetail(ldvalue.String("b"), 2, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v1, user, ldreason.NewEvaluationDetail(ldvalue.String("a"), 1, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v2, user, ldreason.NewEvaluationDetail(ldvalue.String("a"), 1, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag2, user, ldreason.NewEvaluationDetail(ldvalue.String("c"), 3, noReason),
			false, flag2Default, "", ldvalue.OptionalInt{}, false))

		bytes, count := formatter.makeOutputEvents(nil, es.snapshot())
		require.Equal(t, 1, count)

		// Using a nested matcher expression here, rather than an equality assertion on the whole JSON object,
		// because the ordering of array items in "counters" is indeterminate so we need m.ItemsInAnyOrder().
		m.In(t).Assert(bytes, m.JSONArray().Should(m.Items(
			m.MapOf(
				m.KV("kind", m.Equal("summary")),
				m.KV("startDate", m.Not(m.BeNil())),
				m.KV("endDate", m.Not(m.BeNil())),
				m.KV("features", m.MapOf(
					m.KV("flag1", m.MapOf(
						m.KV("default", m.JSONEqual(flag1Default)),
						m.KV("counters", m.ItemsInAnyOrder(
							m.JSONStrEqual(`{"version":100,"variation":1,"value":"a","count":2}`),
							m.JSONStrEqual(`{"version":100,"variation":2,"value":"b","count":1}`),
							m.JSONStrEqual(`{"version":200,"variation":1,"value":"a","count":1}`),
						)),
						m.KV("contextKinds", m.Items(m.Equal("user"))),
					)),
					m.KV("flag2", m.MapOf(
						m.KV("default", m.JSONEqual(flag2Default)),
						m.KV("counters", m.ItemsInAnyOrder(
							m.JSONStrEqual(`{"version":1,"variation":3,"value":"c","count":1}`),
						)),
						m.KV("contextKinds", m.Items(m.Equal("user"))),
					)),
				)),
			),
		)))
	})

	t.Run("summary with multiple context kinds", func(t *testing.T) {
		context1, context2, context3 := ldcontext.New("userkey1"), ldcontext.New("userkey2"), ldcontext.NewWithKind("org", "orgkey")

		es := newEventSummarizer()
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v1, Context(context1), ldreason.NewEvaluationDetail(ldvalue.String("a"), 1, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v1, Context(context2), ldreason.NewEvaluationDetail(ldvalue.String("b"), 2, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v1, Context(context3), ldreason.NewEvaluationDetail(ldvalue.String("a"), 1, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag1v2, Context(context1), ldreason.NewEvaluationDetail(ldvalue.String("a"), 1, noReason),
			false, flag1Default, "", ldvalue.OptionalInt{}, false))
		es.summarizeEvent(withoutReasons.NewEvaluationData(flag2, Context(context1), ldreason.NewEvaluationDetail(ldvalue.String("c"), 3, noReason),
			false, flag2Default, "", ldvalue.OptionalInt{}, false))

		bytes, count := formatter.makeOutputEvents(nil, es.snapshot())
		require.Equal(t, 1, count)

		m.In(t).Assert(bytes, m.JSONArray().Should(m.Items(
			m.MapOf(
				m.KV("kind", m.Equal("summary")),
				m.KV("startDate", m.Not(m.BeNil())),
				m.KV("endDate", m.Not(m.BeNil())),
				m.KV("features", m.MapOf(
					m.KV("flag1", m.MapOf(
						m.KV("default", m.JSONEqual(flag1Default)),
						m.KV("counters", m.Length().Should(m.Equal(3))),
						m.KV("contextKinds", m.ItemsInAnyOrder(m.Equal("user"), m.Equal("org"))),
					)),
					m.KV("flag2", m.MapOf(
						m.KV("default", m.JSONEqual(flag2Default)),
						m.KV("counters", m.Length().Should(m.Equal(1))),
						m.KV("contextKinds", m.Items(m.Equal("user"))),
					)),
				)),
			),
		)))
	})

	t.Run("empty payload", func(t *testing.T) {
		bytes, count := formatter.makeOutputEvents([]anyEventOutput{}, eventSummary{})
		assert.Nil(t, bytes)
		assert.Equal(t, 0, count)
	})
}

func verifyEventOutput(t *testing.T, formatter eventOutputFormatter, event anyEventInput, jsonMatcher m.Matcher) {
	t.Helper()
	bytes, count := formatter.makeOutputEvents([]anyEventOutput{event}, eventSummary{})
	require.Equal(t, 1, count)
	m.In(t).Assert(bytes, m.JSONArray().Should(m.Items(jsonMatcher)))
}

func verifySummaryEventOutput(t *testing.T, formatter eventOutputFormatter, summary eventSummary, jsonMatcher m.Matcher) {
	t.Helper()
	bytes, count := formatter.makeOutputEvents(nil, summary)
	require.Equal(t, 1, count)
	m.In(t).Assert(bytes, m.JSONArray().Should(m.Items(jsonMatcher)))
}

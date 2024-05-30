package ldevents

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
)

func makeEvalEventWithContext(context ldcontext.Context, creationDate ldtime.UnixMillisecondTime, flagKey string,
	flagVersion ldvalue.OptionalInt, variation ldvalue.OptionalInt, value, defaultValue string) EvaluationData {
	return EvaluationData{
		BaseEvent: BaseEvent{CreationDate: creationDate, Context: Context(context)},
		Key:       flagKey,
		Version:   flagVersion,
		Variation: variation,
		Value:     ldvalue.String(value),
		Default:   ldvalue.String(defaultValue),
	}
}

func makeEvalEvent(creationDate ldtime.UnixMillisecondTime, flagKey string,
	flagVersion ldvalue.OptionalInt, variation ldvalue.OptionalInt, value, defaultValue string) EvaluationData {
	return makeEvalEventWithContext(ldcontext.New("key"),
		creationDate, flagKey, flagVersion, variation, value, defaultValue)
}

func TestSummarizeEventSetsStartAndEndDates(t *testing.T) {
	es := newEventSummarizer()
	flagKey := "key"
	event1 := makeEvalEvent(2000, flagKey, ldvalue.NewOptionalInt(1), ldvalue.NewOptionalInt(0), "", "")
	event2 := makeEvalEvent(1000, flagKey, ldvalue.NewOptionalInt(1), ldvalue.NewOptionalInt(0), "", "")
	event3 := makeEvalEvent(1500, flagKey, ldvalue.NewOptionalInt(1), ldvalue.NewOptionalInt(0), "", "")
	es.summarizeEvent(event1)
	es.summarizeEvent(event2)
	es.summarizeEvent(event3)
	data := es.snapshot()

	assert.Equal(t, ldtime.UnixMillisecondTime(1000), data.startDate)
	assert.Equal(t, ldtime.UnixMillisecondTime(2000), data.endDate)
}

func TestSummarizeEventIncrementsCounters(t *testing.T) {
	es := newEventSummarizer()
	flagKey1, flagKey2, unknownFlagKey := "key1", "key2", "badkey"
	flagVersion1, flagVersion2 := ldvalue.NewOptionalInt(11), ldvalue.NewOptionalInt(22)
	variation1, variation2 := ldvalue.NewOptionalInt(1), ldvalue.NewOptionalInt(2)

	event1 := makeEvalEvent(0, flagKey1, flagVersion1, variation1, "value1", "default1")
	event2 := makeEvalEvent(0, flagKey1, flagVersion1, variation2, "value2", "default1")
	event3 := makeEvalEvent(0, flagKey2, flagVersion2, variation1, "value99", "default2")
	event4 := makeEvalEvent(0, flagKey1, flagVersion1, variation1, "value1", "default1")
	event5 := makeEvalEvent(0, unknownFlagKey, undefInt, undefInt, "default3", "default3")
	for _, e := range []EvaluationData{event1, event2, event3, event4, event5} {
		es.summarizeEvent(e)
	}
	data := es.snapshot()

	expectedFlags := map[string]flagSummary{
		flagKey1: {
			defaultValue: ldvalue.String("default1"),
			contextKinds: map[ldcontext.Kind]struct{}{ldcontext.DefaultKind: {}},
			counters: map[counterKey]*counterValue{
				{variation1, flagVersion1}: {2, ldvalue.String("value1")},
				{variation2, flagVersion1}: {1, ldvalue.String("value2")},
			},
		},
		flagKey2: {
			defaultValue: ldvalue.String("default2"),
			contextKinds: map[ldcontext.Kind]struct{}{ldcontext.DefaultKind: {}},
			counters: map[counterKey]*counterValue{
				{variation1, flagVersion2}: {1, ldvalue.String("value99")},
			},
		},
		unknownFlagKey: {
			defaultValue: ldvalue.String("default3"),
			contextKinds: map[ldcontext.Kind]struct{}{ldcontext.DefaultKind: {}},
			counters: map[counterKey]*counterValue{
				{undefInt, undefInt}: {1, ldvalue.String("default3")},
			},
		},
	}
	assert.Equal(t, expectedFlags, data.flags)
}

func TestCounterForNilVariationIsDistinctFromOthers(t *testing.T) {
	es := newEventSummarizer()
	flagKey := "key1"
	flagVersion := ldvalue.NewOptionalInt(11)
	variation1, variation2 := ldvalue.NewOptionalInt(1), ldvalue.NewOptionalInt(2)
	event1 := makeEvalEvent(0, flagKey, flagVersion, variation1, "value1", "default1")
	event2 := makeEvalEvent(0, flagKey, flagVersion, variation2, "value2", "default1")
	event3 := makeEvalEvent(0, flagKey, flagVersion, undefInt, "default1", "default1")
	for _, e := range []EvaluationData{event1, event2, event3} {
		es.summarizeEvent(e)
	}
	data := es.snapshot()

	expectedFlags := map[string]flagSummary{
		flagKey: {
			defaultValue: ldvalue.String("default1"),
			contextKinds: map[ldcontext.Kind]struct{}{ldcontext.DefaultKind: {}},
			counters: map[counterKey]*counterValue{
				{variation1, flagVersion}: {1, ldvalue.String("value1")},
				{variation2, flagVersion}: {1, ldvalue.String("value2")},
				{undefInt, flagVersion}:   {1, ldvalue.String("default1")},
			},
		},
	}
	assert.Equal(t, expectedFlags, data.flags)
}

func TestSummaryContextKindsAreTrackedPerFlag(t *testing.T) {
	es := newEventSummarizer()
	flagKey1, flagKey2 := "key1", "key2"
	flagVersion1, flagVersion2 := ldvalue.NewOptionalInt(11), ldvalue.NewOptionalInt(22)
	variation1, variation2 := ldvalue.NewOptionalInt(1), ldvalue.NewOptionalInt(2)
	context1, context2, context3 := ldcontext.New("userkey1"), ldcontext.New("userkey2"), ldcontext.NewWithKind("org", "orgkey")

	event1 := makeEvalEventWithContext(context1, 0, flagKey1, flagVersion1, variation1, "value1", "default1")
	event2 := makeEvalEventWithContext(context2, 0, flagKey1, flagVersion1, variation2, "value2", "default1")
	event3 := makeEvalEventWithContext(context2, 0, flagKey2, flagVersion2, variation1, "value99", "default2")
	event4 := makeEvalEventWithContext(context3, 0, flagKey1, flagVersion1, variation1, "value1", "default1")
	for _, e := range []EvaluationData{event1, event2, event3, event4} {
		es.summarizeEvent(e)
	}
	data := es.snapshot()

	expectedFlags := map[string]flagSummary{
		flagKey1: {
			defaultValue: ldvalue.String("default1"),
			contextKinds: map[ldcontext.Kind]struct{}{ldcontext.DefaultKind: {}, "org": {}},
			counters: map[counterKey]*counterValue{
				{variation1, flagVersion1}: {2, ldvalue.String("value1")},
				{variation2, flagVersion1}: {1, ldvalue.String("value2")},
			},
		},
		flagKey2: {
			defaultValue: ldvalue.String("default2"),
			contextKinds: map[ldcontext.Kind]struct{}{ldcontext.DefaultKind: {}},
			counters: map[counterKey]*counterValue{
				{variation1, flagVersion2}: {1, ldvalue.String("value99")},
			},
		},
	}
	assert.Equal(t, expectedFlags, data.flags)
}

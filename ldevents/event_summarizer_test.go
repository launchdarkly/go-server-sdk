package ldevents

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

var user = lduser.NewUser("key")

func makeEvalEvent(creationDate ldtime.UnixMillisecondTime, flagKey string, flagVersion int, variation int, value, defaultValue string) FeatureRequestEvent {
	return FeatureRequestEvent{
		BaseEvent: BaseEvent{CreationDate: creationDate, User: user},
		Key:       flagKey,
		Version:   flagVersion,
		Variation: variation,
		Value:     ldvalue.String(value),
		Default:   ldvalue.String(defaultValue),
	}
}

func TestSummarizeEventDoesNothingForIdentifyEvent(t *testing.T) {
	es := newEventSummarizer()
	snapshot := es.snapshot()

	event := defaultEventFactory.NewIdentifyEvent(user)
	es.summarizeEvent(event)

	assert.Equal(t, snapshot, es.snapshot())
}

func TestSummarizeEventDoesNothingForCustomEvent(t *testing.T) {
	es := newEventSummarizer()
	snapshot := es.snapshot()

	event := defaultEventFactory.NewCustomEvent("whatever", user, ldvalue.Null(), false, 0)
	es.summarizeEvent(event)

	assert.Equal(t, snapshot, es.snapshot())
}

func TestSummarizeEventSetsStartAndEndDates(t *testing.T) {
	es := newEventSummarizer()
	flagKey := "key"
	event1 := makeEvalEvent(2000, flagKey, 1, 0, "", "")
	event2 := makeEvalEvent(1000, flagKey, 1, 0, "", "")
	event3 := makeEvalEvent(1500, flagKey, 1, 0, "", "")
	es.summarizeEvent(event1)
	es.summarizeEvent(event2)
	es.summarizeEvent(event3)
	data := es.snapshot()

	assert.Equal(t, ldtime.UnixMillisecondTime(1000), data.startDate)
	assert.Equal(t, ldtime.UnixMillisecondTime(2000), data.endDate)
}

func TestSummarizeEventIncrementsCounters(t *testing.T) {
	es := newEventSummarizer()
	flagKey1 := "key1"
	flagKey2 := "key2"
	flagVersion1 := 11
	flagVersion2 := 22

	unknownFlagKey := "badkey"
	variation1 := 1
	variation2 := 2
	event1 := makeEvalEvent(0, flagKey1, flagVersion1, variation1, "value1", "default1")
	event2 := makeEvalEvent(0, flagKey1, flagVersion1, variation2, "value2", "default1")
	event3 := makeEvalEvent(0, flagKey2, flagVersion2, variation1, "value99", "default2")
	event4 := makeEvalEvent(0, flagKey1, flagVersion1, variation1, "value1", "default1")
	event5 := makeEvalEvent(0, unknownFlagKey, NoVersion, NoVariation, "default3", "default3")
	es.summarizeEvent(event1)
	es.summarizeEvent(event2)
	es.summarizeEvent(event3)
	es.summarizeEvent(event4)
	es.summarizeEvent(event5)
	data := es.snapshot()

	expectedCounters := map[counterKey]*counterValue{
		counterKey{flagKey1, variation1, flagVersion1}:     &counterValue{2, ldvalue.String("value1"), ldvalue.String("default1")},
		counterKey{flagKey1, variation2, flagVersion1}:     &counterValue{1, ldvalue.String("value2"), ldvalue.String("default1")},
		counterKey{flagKey2, variation1, flagVersion2}:     &counterValue{1, ldvalue.String("value99"), ldvalue.String("default2")},
		counterKey{unknownFlagKey, NoVariation, NoVersion}: &counterValue{1, ldvalue.String("default3"), ldvalue.String("default3")},
	}
	assert.Equal(t, expectedCounters, data.counters)
}

func TestCounterForNilVariationIsDistinctFromOthers(t *testing.T) {
	es := newEventSummarizer()
	flagKey := "key1"
	flagVersion := 11
	variation1 := 1
	variation2 := 2
	event1 := makeEvalEvent(0, flagKey, flagVersion, variation1, "value1", "default1")
	event2 := makeEvalEvent(0, flagKey, flagVersion, variation2, "value2", "default1")
	event3 := makeEvalEvent(0, flagKey, flagVersion, NoVariation, "default1", "default1")
	es.summarizeEvent(event1)
	es.summarizeEvent(event2)
	es.summarizeEvent(event3)
	data := es.snapshot()

	expectedCounters := map[counterKey]*counterValue{
		counterKey{flagKey, variation1, flagVersion}: &counterValue{1, ldvalue.String("value1"), ldvalue.String("default1")},
		counterKey{flagKey, variation2, flagVersion}: &counterValue{1, ldvalue.String("value2"), ldvalue.String("default1")},
		counterKey{flagKey, -1, flagVersion}:         &counterValue{1, ldvalue.String("default1"), ldvalue.String("default1")},
	}
	assert.Equal(t, expectedCounters, data.counters)
}

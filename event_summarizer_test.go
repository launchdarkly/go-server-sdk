package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

var user = NewUser("key")

func TestSummarizeEventDoesNothingForIdentifyEvent(t *testing.T) {
	es := newEventSummarizer()
	snapshot := es.snapshot()

	event := NewIdentifyEvent(user)
	es.summarizeEvent(event)

	assert.Equal(t, snapshot, es.snapshot())
}

func TestSummarizeEventDoesNothingForCustomEvent(t *testing.T) {
	es := newEventSummarizer()
	snapshot := es.snapshot()

	event := newCustomEvent("whatever", user, ldvalue.Null(), false, 0)
	es.summarizeEvent(event)

	assert.Equal(t, snapshot, es.snapshot())
}

func TestSummarizeEventSetsStartAndEndDates(t *testing.T) {
	es := newEventSummarizer()
	flag := FeatureFlag{
		Key: "key",
	}
	event1 := newSuccessfulEvalEvent(&flag, user, nil, ldvalue.Null(), ldvalue.Null(), nil, false, nil)
	event2 := newSuccessfulEvalEvent(&flag, user, nil, ldvalue.Null(), ldvalue.Null(), nil, false, nil)
	event3 := newSuccessfulEvalEvent(&flag, user, nil, ldvalue.Null(), ldvalue.Null(), nil, false, nil)
	event1.BaseEvent.CreationDate = 2000
	event2.BaseEvent.CreationDate = 1000
	event3.BaseEvent.CreationDate = 1500
	es.summarizeEvent(event1)
	es.summarizeEvent(event2)
	data := es.snapshot()

	assert.Equal(t, uint64(1000), data.startDate)
	assert.Equal(t, uint64(2000), data.endDate)
}

func TestSummarizeEventIncrementsCounters(t *testing.T) {
	es := newEventSummarizer()
	flag1 := FeatureFlag{
		Key:     "key1",
		Version: 11,
	}
	flag2 := FeatureFlag{
		Key:     "key2",
		Version: 22,
	}
	unknownFlagKey := "badkey"
	variation1 := 1
	variation2 := 2
	event1 := newSuccessfulEvalEvent(&flag1, user, &variation1, ldvalue.String("value1"), ldvalue.String("default1"), nil, false, nil)
	event2 := newSuccessfulEvalEvent(&flag1, user, &variation2, ldvalue.String("value2"), ldvalue.String("default1"), nil, false, nil)
	event3 := newSuccessfulEvalEvent(&flag2, user, &variation1, ldvalue.String("value99"), ldvalue.String("default2"), nil, false, nil)
	event4 := newSuccessfulEvalEvent(&flag1, user, &variation1, ldvalue.String("value1"), ldvalue.String("default1"), nil, false, nil)
	event5 := newUnknownFlagEvent(unknownFlagKey, user, ldvalue.String("default3"), nil, false)
	es.summarizeEvent(event1)
	es.summarizeEvent(event2)
	es.summarizeEvent(event3)
	es.summarizeEvent(event4)
	es.summarizeEvent(event5)
	data := es.snapshot()

	expectedCounters := map[counterKey]*counterValue{
		counterKey{flag1.Key, variation1, flag1.Version}: &counterValue{2, ldvalue.String("value1"), ldvalue.String("default1")},
		counterKey{flag1.Key, variation2, flag1.Version}: &counterValue{1, ldvalue.String("value2"), ldvalue.String("default1")},
		counterKey{flag2.Key, variation1, flag2.Version}: &counterValue{1, ldvalue.String("value99"), ldvalue.String("default2")},
		counterKey{unknownFlagKey, -1, 0}:                &counterValue{1, ldvalue.String("default3"), ldvalue.String("default3")},
	}
	assert.Equal(t, expectedCounters, data.counters)
}

func TestCounterForNilVariationIsDistinctFromOthers(t *testing.T) {
	es := newEventSummarizer()
	flag := FeatureFlag{
		Key:     "key1",
		Version: 11,
	}
	variation1 := 1
	variation2 := 2
	event1 := newSuccessfulEvalEvent(&flag, user, &variation1, ldvalue.String("value1"), ldvalue.String("default1"), nil, false, nil)
	event2 := newSuccessfulEvalEvent(&flag, user, &variation2, ldvalue.String("value2"), ldvalue.String("default1"), nil, false, nil)
	event3 := newSuccessfulEvalEvent(&flag, user, nil, ldvalue.String("default1"), ldvalue.String("default1"), nil, false, nil)
	es.summarizeEvent(event1)
	es.summarizeEvent(event2)
	es.summarizeEvent(event3)
	data := es.snapshot()

	expectedCounters := map[counterKey]*counterValue{
		counterKey{flag.Key, variation1, flag.Version}: &counterValue{1, ldvalue.String("value1"), ldvalue.String("default1")},
		counterKey{flag.Key, variation2, flag.Version}: &counterValue{1, ldvalue.String("value2"), ldvalue.String("default1")},
		counterKey{flag.Key, -1, flag.Version}:         &counterValue{1, ldvalue.String("default1"), ldvalue.String("default1")},
	}
	assert.Equal(t, expectedCounters, data.counters)
}

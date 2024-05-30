package ldevents

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
)

var defaultEventFactory = NewEventFactory(false, nil)

var noReason = ldreason.EvaluationReason{}

func TestEventFactory(t *testing.T) {
	fakeTime := ldtime.UnixMillisecondTime(100000)
	timeFn := func() ldtime.UnixMillisecondTime { return fakeTime }
	withoutReasons := NewEventFactory(false, timeFn)
	withReasons := NewEventFactory(true, timeFn)
	context := Context(ldcontext.New("key"))

	t.Run("NewSuccessfulEvalEvent", func(t *testing.T) {
		flag := FlagEventProperties{Key: "flagkey", Version: 100}

		expected := EvaluationData{
			BaseEvent: BaseEvent{
				CreationDate: fakeTime,
				Context:      context,
			},
			Key:           flag.Key,
			Version:       ldvalue.NewOptionalInt(flag.Version),
			Variation:     ldvalue.NewOptionalInt(1),
			Value:         ldvalue.String("value"),
			Default:       ldvalue.String("default"),
			Reason:        ldreason.NewEvalReasonFallthrough(),
			PrereqOf:      ldvalue.NewOptionalString("pre"),
			SamplingRatio: ldvalue.NewOptionalInt(2),
		}

		event1 := withoutReasons.NewEvaluationData(flag, context,
			ldreason.NewEvaluationDetail(expected.Value, expected.Variation.IntValue(), expected.Reason),
			false, expected.Default, "pre", ldvalue.NewOptionalInt(2), false)
		assert.Equal(t, ldreason.EvaluationReason{}, event1.Reason)
		event1.Reason = expected.Reason
		assert.Equal(t, expected, event1)

		event2 := withReasons.NewEvaluationData(flag, context,
			ldreason.NewEvaluationDetail(expected.Value, expected.Variation.IntValue(), expected.Reason),
			false, expected.Default, "pre", ldvalue.NewOptionalInt(2), false)
		assert.Equal(t, expected, event2)
	})

	t.Run("NewEvaluationData with tracking/debugging", func(t *testing.T) {
		flag := FlagEventProperties{Key: "flagkey", Version: 100}

		expected := EvaluationData{
			BaseEvent: BaseEvent{
				CreationDate: fakeTime,
				Context:      context,
			},
			Key:                flag.Key,
			Version:            ldvalue.NewOptionalInt(flag.Version),
			Variation:          ldvalue.NewOptionalInt(1),
			Value:              ldvalue.String("value"),
			Default:            ldvalue.String("default"),
			SamplingRatio:      ldvalue.NewOptionalInt(2),
		}

		flag1 := flag
		flag1.RequireFullEvent = true
		expected1 := expected
		expected1.RequireFullEvent = true
		event1 := withoutReasons.NewEvaluationData(flag1, context,
			ldreason.NewEvaluationDetail(expected.Value, expected.Variation.IntValue(), ldreason.NewEvalReasonFallthrough()),
			false, expected.Default, "", ldvalue.NewOptionalInt(2), false)
		assert.Equal(t, expected1, event1)

		flag2 := flag
		flag2.DebugEventsUntilDate = ldtime.UnixMillisecondTime(200000)
		expected2 := expected
		expected2.DebugEventsUntilDate = flag2.DebugEventsUntilDate
		event2 := withoutReasons.NewEvaluationData(flag2, context,
			ldreason.NewEvaluationDetail(expected.Value, expected.Variation.IntValue(), ldreason.NewEvalReasonFallthrough()),
			false, expected.Default, "", ldvalue.NewOptionalInt(2), false)
		assert.Equal(t, expected2, event2)
	})

	t.Run("NewEvaluationData with experimentation", func(t *testing.T) {
		flag := FlagEventProperties{Key: "flagkey", Version: 100}

		expected := EvaluationData{
			BaseEvent: BaseEvent{
				CreationDate: fakeTime,
				Context:      context,
			},
			Key:              flag.Key,
			Version:          ldvalue.NewOptionalInt(flag.Version),
			Variation:        ldvalue.NewOptionalInt(1),
			Value:            ldvalue.String("value"),
			Default:          ldvalue.String("default"),
			Reason:           ldreason.NewEvalReasonFallthrough(),
			RequireFullEvent: true,
		}

		event := withoutReasons.NewEvaluationData(flag, context,
			ldreason.NewEvaluationDetail(expected.Value, expected.Variation.IntValue(), ldreason.NewEvalReasonFallthrough()),
			true, expected.Default, "", ldvalue.OptionalInt{}, false)
		assert.Equal(t, expected, event)
	})

	t.Run("NewUnknownFlagEvaluationData", func(t *testing.T) {
		expected := EvaluationData{
			BaseEvent: BaseEvent{
				CreationDate: fakeTime,
				Context:      context,
			},
			Key:     "unknown-key",
			Value:   ldvalue.String("default"),
			Default: ldvalue.String("default"),
			Reason:  ldreason.NewEvalReasonFallthrough(),
		}

		event1 := withoutReasons.NewUnknownFlagEvaluationData(expected.Key, context, expected.Default, expected.Reason)
		assert.Equal(t, ldreason.EvaluationReason{}, event1.Reason)
		event1.Reason = expected.Reason
		assert.Equal(t, expected, event1)
		assert.Equal(t, expected.BaseEvent.CreationDate, event1.CreationDate)

		event2 := withReasons.NewUnknownFlagEvaluationData(expected.Key, context, expected.Default, expected.Reason)
		assert.Equal(t, expected, event2)
	})

	t.Run("NewCustomEvent", func(t *testing.T) {
		expected := CustomEventData{
			BaseEvent: BaseEvent{
				CreationDate: fakeTime,
				Context:      context,
			},
			Key:         "event-key",
			Data:        ldvalue.String("data"),
			HasMetric:   true,
			MetricValue: 2,
		}

		event := withoutReasons.NewCustomEventData(expected.Key, context, expected.Data, true, expected.MetricValue, ldvalue.OptionalInt{})
		assert.Equal(t, expected, event)
		assert.Equal(t, expected.BaseEvent.CreationDate, event.CreationDate)
	})

	t.Run("NewIdentifyEvent", func(t *testing.T) {
		expected := IdentifyEventData{
			BaseEvent: BaseEvent{
				CreationDate: fakeTime,
				Context:      context,
			},
		}

		event := withoutReasons.NewIdentifyEventData(context, ldvalue.OptionalInt{})
		assert.Equal(t, expected, event)
		assert.Equal(t, expected.BaseEvent.CreationDate, event.CreationDate)
	})
}

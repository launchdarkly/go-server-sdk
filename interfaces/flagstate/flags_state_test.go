package flagstate

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
)

func TestAllFlags(t *testing.T) {
	t.Run("IsValid", func(t *testing.T) {
		assert.False(t, AllFlags{}.IsValid())
		assert.True(t, AllFlags{valid: true}.IsValid())
	})

	t.Run("GetFlag", func(t *testing.T) {
		f := FlagState{}
		a := AllFlags{
			flags: map[string]FlagState{"known-flag": f},
		}

		f1, ok := a.GetFlag("known-flag")
		assert.True(t, ok)
		assert.Equal(t, f, f1)

		f2, ok := a.GetFlag("unknown-flag")
		assert.False(t, ok)
		assert.Equal(t, FlagState{}, f2)
	})

	t.Run("GetValue", func(t *testing.T) {
		f := FlagState{Value: ldvalue.String("hi")}
		a := AllFlags{
			flags: map[string]FlagState{"known-flag": f},
		}

		assert.Equal(t, f.Value, a.GetValue("known-flag"))
		assert.Equal(t, ldvalue.Null(), a.GetValue("unknown-flag"))
	})

	t.Run("ToValuesMap", func(t *testing.T) {
		a0 := AllFlags{}
		assert.Len(t, a0.ToValuesMap(), 0)
		assert.NotNil(t, a0.ToValuesMap())

		a1 := AllFlags{
			flags: map[string]FlagState{
				"flag1": {Value: ldvalue.String("value1")},
				"flag2": {Value: ldvalue.String("value2")},
			},
		}
		assert.Equal(t, map[string]ldvalue.Value{
			"flag1": ldvalue.String("value1"),
			"flag2": ldvalue.String("value2"),
		}, a1.ToValuesMap())
	})
}

func TestAllFlagsJSON(t *testing.T) {
	t.Run("invalid state", func(t *testing.T) {
		bytes, err := AllFlags{}.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t, `{"$valid":false,"$flagsState":{}}`, string(bytes))
	})

	t.Run("minimal flag", func(t *testing.T) {
		a := AllFlags{
			valid: true,
			flags: map[string]FlagState{
				"flag1": {
					Value:   ldvalue.String("value1"),
					Version: 1000,
				},
			},
		}
		bytes, err := a.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t,
			`{
  "$valid":true,
  "flag1": "value1",
  "$flagsState":{
    "flag1": {"version":1000}
  }
}`, string(bytes))
	})

	t.Run("flag with all properties except trackReason", func(t *testing.T) {
		a := AllFlags{
			valid: true,
			flags: map[string]FlagState{
				"flag1": {
					Value:                ldvalue.String("value1"),
					Variation:            ldvalue.NewOptionalInt(1),
					Version:              1000,
					Reason:               ldreason.NewEvalReasonFallthrough(),
					TrackEvents:          true,
					DebugEventsUntilDate: ldtime.UnixMillisecondTime(100000),
				},
			},
		}
		bytes, err := a.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t,
			`{
  "$valid":true,
  "flag1": "value1",
  "$flagsState":{
    "flag1": {"variation":1,"version":1000,"reason":{"kind":"FALLTHROUGH"},"trackEvents":true,"debugEventsUntilDate":100000}
  }
}`, string(bytes))
	})

	t.Run("flag with trackReason", func(t *testing.T) {
		a := AllFlags{
			valid: true,
			flags: map[string]FlagState{
				"flag1": {
					Value:       ldvalue.String("value1"),
					Variation:   ldvalue.NewOptionalInt(1),
					Version:     1000,
					Reason:      ldreason.NewEvalReasonFallthrough(),
					TrackEvents: true,
					TrackReason: true,
				},
			},
		}
		bytes, err := a.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t,
			`{
  "$valid":true,
  "flag1": "value1",
  "$flagsState":{
    "flag1": {"variation":1,"version":1000,"reason":{"kind":"FALLTHROUGH"},"trackEvents":true,"trackReason":true}
  }
}`, string(bytes))
	})

	t.Run("omitting details", func(t *testing.T) {
		a := AllFlags{
			valid: true,
			flags: map[string]FlagState{
				"flag1": {
					Value:       ldvalue.String("value1"),
					Variation:   ldvalue.NewOptionalInt(1),
					Version:     1000,
					Reason:      ldreason.NewEvalReasonFallthrough(),
					OmitDetails: true,
				},
			},
		}
		bytes, err := a.MarshalJSON()
		assert.NoError(t, err)
		assert.JSONEq(t,
			`{
  "$valid":true,
  "flag1": "value1",
  "$flagsState":{
    "flag1": {"variation":1}
  }
}`, string(bytes))
	})
}

func TestAllFlagsBuilder(t *testing.T) {
	t.Run("result is always valid", func(t *testing.T) {
		assert.True(t, NewAllFlagsBuilder().Build().IsValid())
	})

	t.Run("add flags without reasons", func(t *testing.T) {
		b := NewAllFlagsBuilder()

		flag1 := FlagState{
			Value:     ldvalue.String("value1"),
			Variation: ldvalue.NewOptionalInt(1),
			Version:   1000,
			Reason:    ldreason.NewEvalReasonFallthrough(),
		}
		flag2 := FlagState{
			Value:                ldvalue.String("value2"),
			Version:              2000,
			Reason:               ldreason.NewEvalReasonError(ldreason.EvalErrorException),
			TrackEvents:          true,
			DebugEventsUntilDate: ldtime.UnixMillisecondTime(100000),
		}
		b.AddFlag("flag1", flag1)
		b.AddFlag("flag2", flag2)

		flag1WithoutReason, flag2WithoutReason := flag1, flag2
		flag1WithoutReason.Reason = ldreason.EvaluationReason{}
		flag2WithoutReason.Reason = ldreason.EvaluationReason{}

		a := b.Build()
		assert.Equal(t, map[string]FlagState{
			"flag1": flag1WithoutReason,
			"flag2": flag2WithoutReason,
		}, a.flags)
	})

	t.Run("add flags with reasons", func(t *testing.T) {
		b := NewAllFlagsBuilder(OptionWithReasons())

		flag1 := FlagState{
			Value:     ldvalue.String("value1"),
			Variation: ldvalue.NewOptionalInt(1),
			Version:   1000,
			Reason:    ldreason.NewEvalReasonFallthrough(),
		}
		flag2 := FlagState{
			Value:                ldvalue.String("value2"),
			Version:              2000,
			Reason:               ldreason.NewEvalReasonError(ldreason.EvalErrorException),
			TrackEvents:          true,
			TrackReason:          true,
			DebugEventsUntilDate: ldtime.UnixMillisecondTime(100000),
		}
		b.AddFlag("flag1", flag1)
		b.AddFlag("flag2", flag2)

		a := b.Build()
		assert.Equal(t, map[string]FlagState{
			"flag1": flag1,
			"flag2": flag2,
		}, a.flags)
	})

	t.Run("add flags with details only if tracked", func(t *testing.T) {
		b := NewAllFlagsBuilder(OptionWithReasons(), OptionDetailsOnlyForTrackedFlags())

		// flag1 should not get full details
		flag1 := FlagState{
			Value:     ldvalue.String("value1"),
			Variation: ldvalue.NewOptionalInt(1),
			Version:   1000,
			Reason:    ldreason.NewEvalReasonFallthrough(),
		}

		// flag2 does not get a reason because, even though DebugEventsUntilDate is set, debugging
		// has already expired (the timestamp is in the past)
		flag2 := FlagState{
			Value:                ldvalue.String("value2"),
			Variation:            ldvalue.NewOptionalInt(2),
			Version:              2000,
			Reason:               ldreason.NewEvalReasonFallthrough(),
			DebugEventsUntilDate: ldtime.UnixMillisecondTime(1),
		}

		// flag3 gets a reason because TrackEvents is true
		flag3 := FlagState{
			Value:       ldvalue.String("value3"),
			Variation:   ldvalue.NewOptionalInt(3),
			Version:     3000,
			Reason:      ldreason.NewEvalReasonRuleMatch(3, "rule3"),
			TrackEvents: true,
		}

		// flag4 gets a reason because DebugEventsUntilDate is set and is in the future
		flag4 := FlagState{
			Value:                ldvalue.String("value4"),
			Variation:            ldvalue.NewOptionalInt(4),
			Version:              4000,
			Reason:               ldreason.NewEvalReasonRuleMatch(4, "rule4"),
			DebugEventsUntilDate: ldtime.UnixMillisNow() + 10000,
		}

		// flag5 gets a reason because TrackReason is true
		flag5 := FlagState{
			Value:       ldvalue.String("value5"),
			Variation:   ldvalue.NewOptionalInt(5),
			Version:     5000,
			Reason:      ldreason.NewEvalReasonRuleMatch(5, "rule5"),
			TrackReason: true,
		}

		b.AddFlag("flag1", flag1)
		b.AddFlag("flag2", flag2)
		b.AddFlag("flag3", flag3)
		b.AddFlag("flag4", flag4)
		b.AddFlag("flag5", flag5)

		flag1WithoutDetails, flag2WithoutDetails := flag1, flag2
		flag1WithoutDetails.OmitDetails = true
		flag2WithoutDetails.OmitDetails = true

		a := b.Build()
		assert.Equal(t, map[string]FlagState{
			"flag1": flag1WithoutDetails,
			"flag2": flag2WithoutDetails,
			"flag3": flag3,
			"flag4": flag4,
			"flag5": flag5,
		}, a.flags)
	})
}

func TestAllFlagsOptions(t *testing.T) {
	assert.Equal(t, "ClientSideOnly", OptionClientSideOnly().String())
	assert.Equal(t, "WithReasons", OptionWithReasons().String())
	assert.Equal(t, "DetailsOnlyForTrackedFlags", OptionDetailsOnlyForTrackedFlags().String())
}

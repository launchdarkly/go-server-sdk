package ldclient

import (
	"errors"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces/flagstate"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
)

func TestAllFlagsStateGetsState(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).
		Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).DebugEventsUntilDate(1000).Build()

	// flag3 has an experiment (evaluation is a fallthrough and TrackEventsFallthrough is on)
	flag3 := ldbuilders.NewFlagBuilder("key3").Version(300).On(true).FallthroughVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value3")).
		TrackEvents(false).TrackEventsFallthrough(true).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag1)
		p.data.UsePreconfiguredFlag(flag2)
		p.data.UsePreconfiguredFlag(flag3)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"))
		assert.True(t, state.IsValid())

		expected := flagstate.NewAllFlagsBuilder().
			AddFlag("key1", flagstate.FlagState{
				Value:     ldvalue.String("value1"),
				Variation: ldvalue.NewOptionalInt(0),
				Version:   100,
			}).
			AddFlag("key2", flagstate.FlagState{
				Value:                ldvalue.String("value2"),
				Variation:            ldvalue.NewOptionalInt(1),
				Version:              200,
				TrackEvents:          true,
				DebugEventsUntilDate: ldtime.UnixMillisecondTime(1000),
			}).
			AddFlag("key3", flagstate.FlagState{
				Value:       ldvalue.String("value3"),
				Variation:   ldvalue.NewOptionalInt(1),
				Version:     300,
				Reason:      ldreason.NewEvalReasonFallthrough(),
				TrackEvents: true,
				TrackReason: true,
			}).
			Build()
		assert.Equal(t, expected, state)
	})
}

func TestAllFlagsStateGetsStateWithReasons(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).On(false).OffVariation(0).
		Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).DebugEventsUntilDate(1000).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag1)
		p.data.UsePreconfiguredFlag(flag2)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), flagstate.OptionWithReasons())
		assert.True(t, state.IsValid())

		expected := flagstate.NewAllFlagsBuilder(flagstate.OptionWithReasons()).
			AddFlag("key1", flagstate.FlagState{
				Value:     ldvalue.String("value1"),
				Variation: ldvalue.NewOptionalInt(0),
				Version:   100,
				Reason:    ldreason.NewEvalReasonOff(),
			}).
			AddFlag("key2", flagstate.FlagState{
				Value:                ldvalue.String("value2"),
				Variation:            ldvalue.NewOptionalInt(1),
				Version:              200,
				Reason:               ldreason.NewEvalReasonOff(),
				TrackEvents:          true,
				DebugEventsUntilDate: ldtime.UnixMillisecondTime(1000),
			}).
			Build()
		assert.Equal(t, expected, state)
	})
}

func TestAllFlagsStateCanFilterForOnlyClientSideFlags(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("server-side-1").Build()
	flag2 := ldbuilders.NewFlagBuilder("server-side-2").Build()
	flag3 := ldbuilders.NewFlagBuilder("client-side-1").SingleVariation(ldvalue.String("value1")).
		ClientSideUsingEnvironmentID(true).Build()
	flag4 := ldbuilders.NewFlagBuilder("client-side-2").SingleVariation(ldvalue.String("value2")).
		ClientSideUsingEnvironmentID(true).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag1)
		p.data.UsePreconfiguredFlag(flag2)
		p.data.UsePreconfiguredFlag(flag3)
		p.data.UsePreconfiguredFlag(flag4)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), flagstate.OptionClientSideOnly())
		assert.True(t, state.IsValid())

		expectedValues := map[string]ldvalue.Value{"client-side-1": ldvalue.String("value1"), "client-side-2": ldvalue.String("value2")}
		assert.Equal(t, expectedValues, state.ToValuesMap())
	})
}

func TestAllFlagsStateCanOmitDetailForUntrackedFlags(t *testing.T) {
	futureTime := ldtime.UnixMillisNow() + 100000

	// flag1 does not get full details because neither event tracking nor debugging is on and there's no experiment
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).Variations(ldvalue.String("value1")).Build()

	// flag2 gets full details because event tracking is on
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).Build()

	// flag3 gets full details because debugging is on
	flag3 := ldbuilders.NewFlagBuilder("key3").Version(300).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value3")).
		TrackEvents(false).DebugEventsUntilDate(futureTime).Build()

	// flag4 gets full details because there's an experiment (evaluation is a fallthrough and TrackEventsFallthrough is on)
	flag4 := ldbuilders.NewFlagBuilder("key4").Version(400).On(true).FallthroughVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value4")).
		TrackEvents(false).TrackEventsFallthrough(true).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag1)
		p.data.UsePreconfiguredFlag(flag2)
		p.data.UsePreconfiguredFlag(flag3)
		p.data.UsePreconfiguredFlag(flag4)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), flagstate.OptionWithReasons(),
			flagstate.OptionDetailsOnlyForTrackedFlags())
		assert.True(t, state.IsValid())

		expected := flagstate.NewAllFlagsBuilder(flagstate.OptionWithReasons()).
			AddFlag("key1", flagstate.FlagState{
				Value:       ldvalue.String("value1"),
				Variation:   ldvalue.NewOptionalInt(0),
				Version:     100,
				Reason:      ldreason.NewEvalReasonOff(),
				OmitDetails: true,
			}).
			AddFlag("key2", flagstate.FlagState{
				Value:       ldvalue.String("value2"),
				Variation:   ldvalue.NewOptionalInt(1),
				Version:     200,
				Reason:      ldreason.NewEvalReasonOff(),
				TrackEvents: true,
			}).
			AddFlag("key3", flagstate.FlagState{
				Value:                ldvalue.String("value3"),
				Variation:            ldvalue.NewOptionalInt(1),
				Version:              300,
				Reason:               ldreason.NewEvalReasonOff(),
				DebugEventsUntilDate: futureTime,
			}).
			AddFlag("key4", flagstate.FlagState{
				Value:       ldvalue.String("value4"),
				Variation:   ldvalue.NewOptionalInt(1),
				Version:     400,
				Reason:      ldreason.NewEvalReasonFallthrough(),
				TrackEvents: true,
				TrackReason: true,
			}).
			Build()
		assert.Equal(t, expected, state)
	})
}

func TestAllFlagsStateReturnsInvalidStateIfClientAndStoreAreNotInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = mocks.DataSourceThatNeverInitializes()
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	state := client.AllFlagsState(evalTestUser)
	assert.False(t, state.IsValid())
	assert.Len(t, state.ToValuesMap(), 0)
}

func TestAllFlagsStateUsesStoreAndLogsWarningIfClientIsNotInitializedButStoreIsInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()
	flag := ldbuilders.NewFlagBuilder(evalFlagKey).SingleVariation(ldvalue.Bool(true)).Build()
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	_ = store.Init(nil)
	_, _ = store.Upsert(datakinds.Features, flag.Key, sharedtest.FlagDescriptor(flag))

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = mocks.DataSourceThatNeverInitializes()
		c.DataStore = mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store}
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	state := client.AllFlagsState(evalTestUser)
	assert.True(t, state.IsValid())
	assert.Len(t, state.ToValuesMap(), 1)

	assert.Len(t, mockLoggers.GetOutput(ldlog.Warn), 1)
	assert.Contains(t, mockLoggers.GetOutput(ldlog.Warn)[0], "using last known values")
}

func TestAllFlagsStateReturnsInvalidStateIfStoreReturnsError(t *testing.T) {
	myError := errors.New("sorry")
	store := mocks.NewCapturingDataStore(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	_ = store.Init(nil)
	store.SetFakeError(myError)
	mockLoggers := ldlogtest.NewMockLog()

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = mocks.DataSourceThatIsAlwaysInitialized()
		c.DataStore = mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store}
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	state := client.AllFlagsState(evalTestUser)
	assert.False(t, state.IsValid())
	assert.Len(t, state.ToValuesMap(), 0)

	assert.Len(t, mockLoggers.GetOutput(ldlog.Warn), 1)
	assert.Contains(t, mockLoggers.GetOutput(ldlog.Warn)[0], "Unable to fetch flags")
}

type test struct {
	name    string
	options []flagstate.Option
}

func optionPermutations() []test {
	type option struct {
		name   string
		option flagstate.Option
	}
	options := []option{
		{name: "with reasons", option: flagstate.OptionWithReasons()},
		{name: "client-side only", option: flagstate.OptionClientSideOnly()},
		{name: "details only for tracked flags", option: flagstate.OptionDetailsOnlyForTrackedFlags()},
	}
	tests := []test{
		{name: "no options", options: []flagstate.Option{}},
	}
	for i, opt := range options {
		tests = append(tests, test{name: opt.name, options: []flagstate.Option{opt.option}})
		for j := i + 1; j < len(options); j++ {
			tests = append(tests, test{name: opt.name + " and " + options[j].name, options: []flagstate.Option{opt.option, options[j].option}})
			for k := j + 1; k < len(options); k++ {
				tests = append(tests, test{name: opt.name + " and " + options[j].name + " and " + options[k].name,
					options: []flagstate.Option{opt.option, options[j].option, options[k].option}})
			}
		}
	}

	return tests
}

func TestAllFlagsStateReturnsPrerequisites(t *testing.T) {

	t.Run("when flag is visible to clients", func(t *testing.T) {
		flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).On(true).OffVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key2", 0).ClientSideUsingEnvironmentID(true).Build()

		flag2 := ldbuilders.NewFlagBuilder("key2").Version(100).OffVariation(0).
			Variations(ldvalue.String("value1")).ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		for _, test := range optionPermutations() {
			t.Run(test.name, func(t *testing.T) {
				withClientEvalTestParams(func(p clientEvalTestParams) {
					p.data.UsePreconfiguredFlag(flag1)
					p.data.UsePreconfiguredFlag(flag2)

					state := p.client.AllFlagsState(lduser.NewUser("userkey"), test.options...)
					assert.True(t, state.IsValid())

					flag1state, ok := state.GetFlag("key1")
					assert.True(t, ok)
					assert.Equal(t, []string{"key2"}, flag1state.Prerequisites)
				})
			})
		}
	})

	t.Run("when flag is not visible to clients", func(t *testing.T) {
		flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).On(true).OffVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key2", 0).
			ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		flag2 := ldbuilders.NewFlagBuilder("key2").Version(100).OffVariation(0).
			Variations(ldvalue.String("value1")).ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		for _, test := range optionPermutations() {
			t.Run(test.name, func(t *testing.T) {
				withClientEvalTestParams(func(p clientEvalTestParams) {
					p.data.UsePreconfiguredFlag(flag1)
					p.data.UsePreconfiguredFlag(flag2)

					state := p.client.AllFlagsState(lduser.NewUser("userkey"), test.options...)
					assert.True(t, state.IsValid())

					// If the flag was visible, then we should see its prerequisites.
					fs1, ok := state.GetFlag("key1")
					if ok {
						assert.Equal(t, []string{"key2"}, fs1.Prerequisites)
					}
				})
			})
		}
	})

	t.Run("when flag is off, no prerequisites are returned", func(t *testing.T) {
		flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).On(false).OffVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key2", 0).
			ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		flag2 := ldbuilders.NewFlagBuilder("key2").Version(100).OffVariation(0).
			Variations(ldvalue.String("value1")).ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		for _, test := range optionPermutations() {
			t.Run(test.name, func(t *testing.T) {
				withClientEvalTestParams(func(p clientEvalTestParams) {
					p.data.UsePreconfiguredFlag(flag1)
					p.data.UsePreconfiguredFlag(flag2)

					state := p.client.AllFlagsState(lduser.NewUser("userkey"), test.options...)
					assert.True(t, state.IsValid())

					// If the flag was visible, then we should see that it had no prerequisites evaluated
					// since the flag was off.
					fs1, ok := state.GetFlag("key1")
					if ok {
						assert.Empty(t, fs1.Prerequisites)
					}
				})
			})
		}
	})

	t.Run("only returns top-level prerequisites", func(t *testing.T) {
		flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).On(true).OffVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key2", 0).
			ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		flag2 := ldbuilders.NewFlagBuilder("key2").Version(100).On(true).OffVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key3", 0).
			ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		flag3 := ldbuilders.NewFlagBuilder("key3").Version(100).On(false).OffVariation(0).
			Variations(ldvalue.String("value1")).ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		for _, test := range optionPermutations() {
			t.Run(test.name, func(t *testing.T) {
				withClientEvalTestParams(func(p clientEvalTestParams) {
					p.data.UsePreconfiguredFlag(flag1)
					p.data.UsePreconfiguredFlag(flag2)
					p.data.UsePreconfiguredFlag(flag3)

					state := p.client.AllFlagsState(lduser.NewUser("userkey"), test.options...)
					assert.True(t, state.IsValid())

					// If the flag was visible, then we should see that it had no prerequisites evaluated
					// since the flag was off.
					fs1, ok := state.GetFlag("key1")
					if ok {
						assert.Equal(t, []string{"key2"}, fs1.Prerequisites)
					}

					fs2, ok := state.GetFlag("key2")
					if ok {
						assert.Equal(t, []string{"key3"}, fs2.Prerequisites)
					}

					fs3, ok := state.GetFlag("key3")
					if ok {
						assert.Empty(t, fs3.Prerequisites)
					}
				})
			})
		}
	})

	t.Run("prerequisites are in evaluation order", func(t *testing.T) {
		ascending := ldbuilders.NewFlagBuilder("key1").Version(100).On(true).OffVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key2", 0).AddPrerequisite("key3", 0).
			ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		descending := ldbuilders.NewFlagBuilder("key1").Version(100).On(true).OffVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key3", 0).AddPrerequisite("key2", 0).
			ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		flag2 := ldbuilders.NewFlagBuilder("key2").Version(100).On(true).OffVariation(0).FallthroughVariation(0).
			Variations(ldvalue.String("value1")).AddPrerequisite("key3", 0).
			ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		flag3 := ldbuilders.NewFlagBuilder("key3").Version(100).On(true).OffVariation(0).FallthroughVariation(0).
			Variations(ldvalue.String("value1")).ClientSideUsingEnvironmentID(false).ClientSideUsingMobileKey(false).Build()

		t.Run("ascending", func(t *testing.T) {
			for _, test := range optionPermutations() {
				t.Run(test.name, func(t *testing.T) {
					withClientEvalTestParams(func(p clientEvalTestParams) {
						p.data.UsePreconfiguredFlag(ascending)
						p.data.UsePreconfiguredFlag(flag2)
						p.data.UsePreconfiguredFlag(flag3)

						state := p.client.AllFlagsState(lduser.NewUser("userkey"), test.options...)
						assert.True(t, state.IsValid())

						fs1, ok := state.GetFlag("key1")
						if ok {
							assert.Equal(t, []string{"key2", "key3"}, fs1.Prerequisites)
						}
					})
				})
			}
		})

		t.Run("descending", func(t *testing.T) {
			for _, test := range optionPermutations() {
				t.Run(test.name, func(t *testing.T) {
					withClientEvalTestParams(func(p clientEvalTestParams) {
						p.data.UsePreconfiguredFlag(descending)
						p.data.UsePreconfiguredFlag(flag2)
						p.data.UsePreconfiguredFlag(flag3)

						state := p.client.AllFlagsState(lduser.NewUser("userkey"), test.options...)
						assert.True(t, state.IsValid())

						fs1, ok := state.GetFlag("key1")
						if ok {
							assert.Equal(t, []string{"key3", "key2"}, fs1.Prerequisites)
						}
					})
				})
			}
		})
	})

}

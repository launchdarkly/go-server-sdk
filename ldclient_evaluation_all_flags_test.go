package ldclient

import (
	"errors"
	"github.com/stretchr/testify/require"
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

func TestAllFlagsStateReturnsPrerequisites(t *testing.T) {

	// Creates a boolean flag that is on (true).
	booleanFlag := func(key string) *ldbuilders.FlagBuilder {
		return ldbuilders.NewFlagBuilder(key).
			Version(100).
			On(true).
			OffVariation(0).
			FallthroughVariation(1).
			Variations(ldvalue.Bool(false), ldvalue.Bool(true))
	}

	t.Run("only top-level (direct) prerequisites are included in the prerequisites field", func(t *testing.T) {

		// Toplevel has one direct prerequisite.
		toplevel := booleanFlag("toplevel").
			AddPrerequisite("prereq1", 1).Build()

		// Prereq1 also has one direct prerequisite.
		prereq1 := booleanFlag("prereq1").
			AddPrerequisite("prereq2", 1).Build()

		// Prereq2 has no prerequisites itself.
		prereq2 := booleanFlag("prereq2").Build()

		// We expect that toplevel and prereq1 should list only their direct prerequisites. That is,
		// toplevel should not list [prereq1, prereq2], but only [prereq1].

		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(toplevel)
			p.data.UsePreconfiguredFlag(prereq1)
			p.data.UsePreconfiguredFlag(prereq2)

			state := p.client.AllFlagsState(lduser.NewUser("userkey"))
			require.True(t, state.IsValid())

			toplevelState, ok := state.GetFlag("toplevel")
			if assert.True(t, ok) {
				assert.Equal(t, []string{"prereq1"}, toplevelState.Prerequisites)
			}

			prereq1State, ok := state.GetFlag("prereq1")
			if assert.True(t, ok) {
				assert.Equal(t, []string{"prereq2"}, prereq1State.Prerequisites)
			}

			prereq2State, ok := state.GetFlag("prereq2")
			if assert.True(t, ok) {
				assert.Empty(t, prereq2State.Prerequisites)
			}
		})
	})

	t.Run("the prerequisites field should hold prerequisites in evaluation order", func(t *testing.T) {

		// These tests ensure all direct prerequisites of a flag (that is on) are included in the
		// prerequisites field. The sub-tests are a sanity check to make sure we're not sorting the array
		// accidentally - the array should follow eval order.
		t.Run("depth one, all on", func(t *testing.T) {
			t.Run("ascending alphabetic", func(t *testing.T) {
				toplevel := booleanFlag("toplevel").
					AddPrerequisite("prereq1", 1).
					AddPrerequisite("prereq2", 1).
					AddPrerequisite("prereq3", 1).Build()

				prereq1 := booleanFlag("prereq1").Build()
				prereq2 := booleanFlag("prereq2").Build()
				prereq3 := booleanFlag("prereq3").Build()

				withClientEvalTestParams(func(p clientEvalTestParams) {
					p.data.UsePreconfiguredFlag(toplevel)
					p.data.UsePreconfiguredFlag(prereq1)
					p.data.UsePreconfiguredFlag(prereq2)
					p.data.UsePreconfiguredFlag(prereq3)

					state := p.client.AllFlagsState(lduser.NewUser("userkey"))
					require.True(t, state.IsValid())

					toplevelState, ok := state.GetFlag("toplevel")
					if assert.True(t, ok) {
						assert.Equal(t, []string{"prereq1", "prereq2", "prereq3"}, toplevelState.Prerequisites)
					}
				})
			})

			t.Run("descending alphabetic", func(t *testing.T) {
				toplevel := booleanFlag("toplevel").
					AddPrerequisite("prereq3", 1).
					AddPrerequisite("prereq2", 1).
					AddPrerequisite("prereq1", 1).Build()

				prereq1 := booleanFlag("prereq1").Build()
				prereq2 := booleanFlag("prereq2").Build()
				prereq3 := booleanFlag("prereq3").Build()

				withClientEvalTestParams(func(p clientEvalTestParams) {
					p.data.UsePreconfiguredFlag(toplevel)
					p.data.UsePreconfiguredFlag(prereq1)
					p.data.UsePreconfiguredFlag(prereq2)
					p.data.UsePreconfiguredFlag(prereq3)

					state := p.client.AllFlagsState(lduser.NewUser("userkey"))
					require.True(t, state.IsValid())

					toplevelState, ok := state.GetFlag("toplevel")
					if assert.True(t, ok) {
						assert.Equal(t, []string{"prereq3", "prereq2", "prereq1"}, toplevelState.Prerequisites)
					}
				})
			})
		})

		t.Run("depth three, toplevel flag is off", func(t *testing.T) {
			toplevel := booleanFlag("toplevel").
				AddPrerequisite("prereq1", 1).On(false).Build()

			prereq1 := booleanFlag("prereq1").
				AddPrerequisite("prereq2", 1).Build()

			prereq2 := booleanFlag("prereq2").
				AddPrerequisite("prereq3", 1).Build()

			prereq3 := booleanFlag("prereq3").Build()

			withClientEvalTestParams(func(p clientEvalTestParams) {
				p.data.UsePreconfiguredFlag(toplevel)
				p.data.UsePreconfiguredFlag(prereq1)
				p.data.UsePreconfiguredFlag(prereq2)
				p.data.UsePreconfiguredFlag(prereq3)

				state := p.client.AllFlagsState(lduser.NewUser("userkey"))
				require.True(t, state.IsValid())

				// If toplevel were on, then we'd expect to see the single prerequisite. Since it's not, we shouldn't
				// see any.
				toplevelState, ok := state.GetFlag("toplevel")
				if assert.True(t, ok) {
					assert.Empty(t, toplevelState.Prerequisites)
				}

				// The other prerequisites are themselves top-level flags when evaluated, so they should have their
				// own prerequisites included since they're all on.
				prereq1State, ok := state.GetFlag("prereq1")
				if assert.True(t, ok) {
					assert.Equal(t, []string{"prereq2"}, prereq1State.Prerequisites)
				}

				prereq2State, ok := state.GetFlag("prereq2")
				if assert.True(t, ok) {
					assert.Equal(t, []string{"prereq3"}, prereq2State.Prerequisites)
				}

				// Prereq1 had no prereqs.
				prereq3State, ok := state.GetFlag("prereq3")
				if assert.True(t, ok) {
					assert.Empty(t, prereq3State.Prerequisites)
				}
			})
		})

		t.Run("depth three, first prerequisite is off", func(t *testing.T) {
			toplevel := booleanFlag("toplevel").
				AddPrerequisite("prereq1", 1).Build()

			prereq1 := booleanFlag("prereq1").
				AddPrerequisite("prereq2", 1).On(false).Build()

			prereq2 := booleanFlag("prereq2").
				AddPrerequisite("prereq3", 1).Build()

			prereq3 := booleanFlag("prereq3").Build()

			withClientEvalTestParams(func(p clientEvalTestParams) {
				p.data.UsePreconfiguredFlag(toplevel)
				p.data.UsePreconfiguredFlag(prereq1)
				p.data.UsePreconfiguredFlag(prereq2)
				p.data.UsePreconfiguredFlag(prereq3)

				state := p.client.AllFlagsState(lduser.NewUser("userkey"))
				require.True(t, state.IsValid())

				toplevelState, ok := state.GetFlag("toplevel")
				if assert.True(t, ok) {
					assert.Equal(t, []string{"prereq1"}, toplevelState.Prerequisites)
				}

				prereq1State, ok := state.GetFlag("prereq1")
				if assert.True(t, ok) {
					assert.Empty(t, prereq1State.Prerequisites)
				}

				prereq2State, ok := state.GetFlag("prereq2")
				if assert.True(t, ok) {
					assert.Equal(t, []string{"prereq3"}, prereq2State.Prerequisites)
				}

				prereq3State, ok := state.GetFlag("prereq3")
				if assert.True(t, ok) {
					assert.Empty(t, prereq3State.Prerequisites)
				}
			})
		})

		t.Run("depth three, second  prerequisite is off", func(t *testing.T) {
			toplevel := booleanFlag("toplevel").
				AddPrerequisite("prereq1", 1).Build()

			prereq1 := booleanFlag("prereq1").
				AddPrerequisite("prereq2", 1).Build()

			prereq2 := booleanFlag("prereq2").
				AddPrerequisite("prereq3", 1).On(false).Build()

			prereq3 := booleanFlag("prereq3").Build()

			withClientEvalTestParams(func(p clientEvalTestParams) {
				p.data.UsePreconfiguredFlag(toplevel)
				p.data.UsePreconfiguredFlag(prereq1)
				p.data.UsePreconfiguredFlag(prereq2)
				p.data.UsePreconfiguredFlag(prereq3)

				state := p.client.AllFlagsState(lduser.NewUser("userkey"))
				require.True(t, state.IsValid())

				toplevelState, ok := state.GetFlag("toplevel")
				if assert.True(t, ok) {
					assert.Equal(t, []string{"prereq1"}, toplevelState.Prerequisites)
				}

				prereq1State, ok := state.GetFlag("prereq1")
				if assert.True(t, ok) {
					assert.Equal(t, []string{"prereq2"}, prereq1State.Prerequisites)
				}

				prereq2State, ok := state.GetFlag("prereq2")
				if assert.True(t, ok) {
					assert.Empty(t, prereq2State.Prerequisites)
				}

				prereq3State, ok := state.GetFlag("prereq3")
				if assert.True(t, ok) {
					assert.Empty(t, prereq3State.Prerequisites)
				}
			})
		})

	})

	t.Run("prerequisite flags not visible to client SDKs should still be referenced in visible flags", func(t *testing.T) {
		toplevel := booleanFlag("toplevel").
			ClientSideUsingEnvironmentID(true).
			AddPrerequisite("prereq1", 1).
			AddPrerequisite("prereq2", 1).Build()

		prereq1 := booleanFlag("prereq1").
			ClientSideUsingEnvironmentID(false).Build()

		prereq2 := booleanFlag("prereq2").
			ClientSideUsingEnvironmentID(false).Build()

		withClientEvalTestParams(func(p clientEvalTestParams) {
			p.data.UsePreconfiguredFlag(toplevel)
			p.data.UsePreconfiguredFlag(prereq1)
			p.data.UsePreconfiguredFlag(prereq2)

			state := p.client.AllFlagsState(lduser.NewUser("userkey"), flagstate.OptionClientSideOnly())
			require.True(t, state.IsValid())

			toplevelState, ok := state.GetFlag("toplevel")
			if assert.True(t, ok) {
				assert.Equal(t, []string{"prereq1", "prereq2"}, toplevelState.Prerequisites)
			}

			_, ok = state.GetFlag("prereq1")
			assert.False(t, ok)

			_, ok = state.GetFlag("prereq2")
			assert.False(t, ok)
		})
	})
}

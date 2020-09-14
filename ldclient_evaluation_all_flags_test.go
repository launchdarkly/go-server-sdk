package ldclient

import (
	"errors"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/flagstate"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"github.com/stretchr/testify/assert"
)

func TestAllFlagsStateGetsState(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).
		Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).DebugEventsUntilDate(1000).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag1)
		p.data.UsePreconfiguredFlag(flag2)

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
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).Build()
	flag3 := ldbuilders.NewFlagBuilder("key3").Version(300).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value3")).
		TrackEvents(false).DebugEventsUntilDate(futureTime).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		p.data.UsePreconfiguredFlag(flag1)
		p.data.UsePreconfiguredFlag(flag2)
		p.data.UsePreconfiguredFlag(flag3)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), flagstate.OptionWithReasons(),
			flagstate.OptionDetailsOnlyForTrackedFlags())
		assert.True(t, state.IsValid())

		expected := flagstate.NewAllFlagsBuilder(flagstate.OptionWithReasons()).
			AddFlag("key1", flagstate.FlagState{
				Value:     ldvalue.String("value1"),
				Variation: ldvalue.NewOptionalInt(0),
				Version:   100,
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
			Build()
		assert.Equal(t, expected, state)
	})
}

func TestAllFlagsStateReturnsInvalidStateIfClientAndStoreAreNotInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = sharedtest.DataSourceThatNeverInitializes()
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
	_, _ = store.Upsert(datakinds.Features, flag.GetKey(), sharedtest.FlagDescriptor(flag))

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = sharedtest.DataSourceThatNeverInitializes()
		c.DataStore = sharedtest.SingleDataStoreFactory{Instance: store}
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
	store := sharedtest.NewCapturingDataStore(datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()))
	_ = store.Init(nil)
	store.SetFakeError(myError)
	mockLoggers := ldlogtest.NewMockLog()

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = sharedtest.DataSourceThatIsAlwaysInitialized()
		c.DataStore = sharedtest.SingleDataStoreFactory{Instance: store}
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	state := client.AllFlagsState(evalTestUser)
	assert.False(t, state.IsValid())
	assert.Len(t, state.ToValuesMap(), 0)

	assert.Len(t, mockLoggers.GetOutput(ldlog.Warn), 1)
	assert.Contains(t, mockLoggers.GetOutput(ldlog.Warn)[0], "Unable to fetch flags")
}

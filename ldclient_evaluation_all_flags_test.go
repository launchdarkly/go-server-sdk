package ldclient

import (
	"errors"
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/flagstate"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func TestAllFlagsStateGetsState(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).
		Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).DebugEventsUntilDate(1000).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag1)
		sharedtest.UpsertFlag(p.store, &flag2)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"))
		assert.True(t, state.IsValid())

		expected := flagstate.NewAllFlagsBuilder().
			AddFlag("key1", flagstate.FlagState{
				Value:     ldvalue.String("value1"),
				Variation: 0,
				Version:   100,
			}).
			AddFlag("key2", flagstate.FlagState{
				Value:                ldvalue.String("value2"),
				Variation:            1,
				Version:              200,
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
	flag3 := ldbuilders.NewFlagBuilder("client-side-1").SingleVariation(ldvalue.String("value1")).ClientSide(true).Build()
	flag4 := ldbuilders.NewFlagBuilder("client-side-2").SingleVariation(ldvalue.String("value2")).ClientSide(true).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag1)
		sharedtest.UpsertFlag(p.store, &flag2)
		sharedtest.UpsertFlag(p.store, &flag3)
		sharedtest.UpsertFlag(p.store, &flag4)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), flagstate.OptionClientSideOnly())
		assert.True(t, state.IsValid())

		expectedValues := map[string]ldvalue.Value{"client-side-1": ldvalue.String("value1"), "client-side-2": ldvalue.String("value2")}
		assert.Equal(t, expectedValues, state.ToValuesMap())
	})
}

func TestAllFlagsStateGetsStateWithReasons(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).On(false).OffVariation(0).
		Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).On(true).FallthroughVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value2")).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag1)
		sharedtest.UpsertFlag(p.store, &flag2)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), flagstate.OptionWithReasons())
		assert.True(t, state.IsValid())

		expected := flagstate.NewAllFlagsBuilder(flagstate.OptionWithReasons()).
			AddFlag("key1", flagstate.FlagState{
				Value:     ldvalue.String("value1"),
				Variation: 0,
				Version:   100,
				Reason:    ldreason.NewEvalReasonOff(),
			}).
			AddFlag("key2", flagstate.FlagState{
				Value:     ldvalue.String("value2"),
				Variation: 1,
				Version:   200,
				Reason:    ldreason.NewEvalReasonFallthrough(),
			}).
			Build()
		assert.Equal(t, expected, state)
	})
}

func TestAllFlagsStateReturnsInvalidStateIfClientAndStoreAreNotInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: false},
		}
		c.Logging = ldcomponents.Logging().Loggers(mockLoggers.Loggers)
	})
	defer client.Close()

	state := client.AllFlagsState(evalTestUser)
	assert.False(t, state.IsValid())
	assert.Len(t, state.ToValuesMap(), 0)
}

func TestAllFlagsStateUsesStoreAndLogsWarningIfClientIsNotInitializedButStoreIsInitialized(t *testing.T) {
	mockLoggers := ldlogtest.NewMockLog()
	flag := singleValueFlag("flagkey", ldvalue.Bool(true))
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	_ = store.Init(nil)
	_, _ = store.Upsert(datakinds.Features, flag.GetKey(), sharedtest.FlagDescriptor(flag))

	client := makeTestClientWithConfig(func(c *Config) {
		c.DataSource = sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: false},
		}
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
		c.DataSource = sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: true},
		}
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

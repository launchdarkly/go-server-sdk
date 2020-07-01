package ldclient

import (
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
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

		expectedString := `{
			"key1":"value1",
			"key2":"value2",
			"$flagsState":{
				"key1":{
					"variation":0,"version":100,"reason":null
				},
				"key2": {
					"variation":1,"version":200,"trackEvents":true,"debugEventsUntilDate":1000,"reason":null
				}
			},
			"$valid":true
		}`
		actualBytes, err := json.Marshal(state)
		assert.NoError(t, err)
		assert.JSONEq(t, expectedString, string(actualBytes))
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

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), ClientSideOnly)
		assert.True(t, state.IsValid())

		expectedValues := map[string]ldvalue.Value{"client-side-1": ldvalue.String("value1"), "client-side-2": ldvalue.String("value2")}
		assert.Equal(t, expectedValues, state.ToValuesMap())
	})
}

func TestAllFlagsStateGetsStateWithReasons(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).
		Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).
		Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).DebugEventsUntilDate(1000).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag1)
		sharedtest.UpsertFlag(p.store, &flag2)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), WithReasons)
		assert.True(t, state.IsValid())

		expectedString := `{
			"key1":"value1",
			"key2":"value2",
			"$flagsState":{
				"key1":{
					"variation":0,"version":100,"reason":{"kind":"OFF"}
				},
				"key2": {
					"variation":1,"version":200,"reason":{"kind":"OFF"},"trackEvents":true,"debugEventsUntilDate":1000
				}
			},
			"$valid":true
		}`
		actualBytes, err := json.Marshal(state)
		assert.NoError(t, err)
		assert.JSONEq(t, expectedString, string(actualBytes))
	})
}

func TestAllFlagsStateCanOmitDetailForUntrackedFlags(t *testing.T) {
	futureTime := ldtime.UnixMillisNow() + 100000
	futureTimeStr := strconv.FormatInt(int64(futureTime), 10)
	flag1 := ldbuilders.NewFlagBuilder("key1").Version(100).OffVariation(0).Variations(ldvalue.String("value1")).Build()
	flag2 := ldbuilders.NewFlagBuilder("key2").Version(200).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value2")).
		TrackEvents(true).Build()
	flag3 := ldbuilders.NewFlagBuilder("key3").Version(300).OffVariation(1).Variations(ldvalue.String("x"), ldvalue.String("value3")).
		TrackEvents(false).DebugEventsUntilDate(futureTime).Build()

	withClientEvalTestParams(func(p clientEvalTestParams) {
		sharedtest.UpsertFlag(p.store, &flag1)
		sharedtest.UpsertFlag(p.store, &flag2)
		sharedtest.UpsertFlag(p.store, &flag3)

		state := p.client.AllFlagsState(lduser.NewUser("userkey"), WithReasons, DetailsOnlyForTrackedFlags)
		assert.True(t, state.IsValid())

		expectedString := `{
			"key1":"value1",
			"key2":"value2",
			"key3":"value3",
			"$flagsState":{
				"key1":{
					"variation":0
				},
				"key2": {
					"variation":1,"version":200,"reason":{"kind":"OFF"},"trackEvents":true
				},
				"key3": {
					"variation":1,"version":300,"reason":{"kind":"OFF"},"debugEventsUntilDate":` + futureTimeStr + `
				}
			},
			"$valid":true
		}`
		actualBytes, err := json.Marshal(state)
		assert.NoError(t, err)
		assert.JSONEq(t, expectedString, string(actualBytes))
	})
}

func TestAllFlagsStateReturnsInvalidStateIfClientAndStoreAreNotInitialized(t *testing.T) {
	mockLoggers := sharedtest.NewMockLoggers()

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
	mockLoggers := sharedtest.NewMockLoggers()
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
	mockLoggers := sharedtest.NewMockLoggers()

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

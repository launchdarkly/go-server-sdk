package ldclient

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"gopkg.in/launchdarkly/go-server-sdk.v5/lddynamodb"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest/dynamodbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldconsul"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldredis"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

// This is a smoke test of initializing LDClient with any of the built-in persistent data store
// implementations. We test those separately in their own packages, but here we verify that the other
// SDK components are interacting with them correctly. Caching is disabled so that every SDK operation
// hits the database.

func testClientWithPersistentStore(t *testing.T, factory interfaces.PersistentDataStoreFactory) {
	flagKey, segmentKey, userKey, otherUserKey := "flagkey", "segmentkey", "userkey", "otheruser"
	goodValue1, goodValue2, badValue := ldvalue.String("good"), ldvalue.String("better"), ldvalue.String("bad")
	goodVariation1, goodVariation2, badVariation := 0, 1, 2
	user, otherUser := lduser.NewUser(userKey), lduser.NewUser(otherUserKey)

	makeFlagThatReturnsVariationForSegmentMatch := func(version int, variation int) ldmodel.FeatureFlag {
		return ldbuilders.NewFlagBuilder(flagKey).Version(version).
			On(true).
			Variations(goodValue1, goodValue2, badValue).
			FallthroughVariation(badVariation).
			AddRule(ldbuilders.NewRuleBuilder().Variation(variation).Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segmentKey)),
			)).
			Build()
	}
	makeSegmentThatMatchesUserKeys := func(version int, keys ...string) ldmodel.Segment {
		return ldbuilders.NewSegmentBuilder(segmentKey).Version(version).
			Included(keys...).
			Build()
	}
	flag := makeFlagThatReturnsVariationForSegmentMatch(1, goodVariation1)
	segment := makeSegmentThatMatchesUserKeys(1, userKey)

	data := []interfaces.StoreCollection{
		{Kind: interfaces.DataKindFeatures(), Items: []interfaces.StoreKeyedItemDescriptor{
			{Key: flagKey, Item: sharedtest.FlagDescriptor(flag)},
		}},
		{Kind: interfaces.DataKindSegments(), Items: []interfaces.StoreKeyedItemDescriptor{
			{Key: segmentKey, Item: sharedtest.SegmentDescriptor(segment)},
		}},
	}
	dataSourceFactory := &dataSourceFactoryThatExposesUpdater{ // allows us to simulate an update
		underlyingFactory: dataSourceFactoryWithData{data: data},
	}
	mockLog := sharedtest.NewMockLoggers()
	config := Config{
		DataStore:  ldcomponents.PersistentDataStore(factory).NoCaching(),
		DataSource: dataSourceFactory,
		Events:     ldcomponents.NoEvents(),
		Logging:    ldcomponents.Logging().Loggers(mockLog.Loggers),
	}

	client, err := MakeCustomClient(testSdkKey, config, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	flagShouldHaveValueForUser := func(u lduser.User, expectedValue ldvalue.Value) {
		value, err := client.JSONVariation(flagKey, u, ldvalue.Null())
		assert.NoError(t, err)
		assert.Equal(t, expectedValue, value)
	}

	// verify that the client can get the flag from the store and evaluate it
	flagShouldHaveValueForUser(user, goodValue1)
	flagShouldHaveValueForUser(otherUser, badValue)

	// verify that the client can get all flags
	state := client.AllFlagsState(user)
	assert.Equal(t, map[string]ldvalue.Value{flagKey: goodValue1}, state.ToValuesMap())

	// verify that an update is persisted
	flagv2 := makeFlagThatReturnsVariationForSegmentMatch(2, goodVariation2)
	dataSourceFactory.dataSourceUpdates.Upsert(interfaces.DataKindFeatures(), flagKey,
		sharedtest.FlagDescriptor(flagv2))

	flagShouldHaveValueForUser(user, goodValue2)
	flagShouldHaveValueForUser(otherUser, badValue)

	segmentv2 := makeSegmentThatMatchesUserKeys(2, userKey, otherUserKey)
	dataSourceFactory.dataSourceUpdates.Upsert(interfaces.DataKindSegments(), segmentKey,
		sharedtest.SegmentDescriptor(segmentv2))
	flagShouldHaveValueForUser(otherUser, goodValue2) // otherUser is now matched by the segment

	// verify that a deletion is persisted
	// deleting the segment should cause the flag that uses it to stop matching
	dataSourceFactory.dataSourceUpdates.Upsert(interfaces.DataKindSegments(), segmentKey,
		interfaces.StoreItemDescriptor{Version: 3, Item: nil})
	flagShouldHaveValueForUser(user, badValue)

	// deleting the flag should cause the flag to become unknown
	dataSourceFactory.dataSourceUpdates.Upsert(interfaces.DataKindFeatures(), flagKey,
		interfaces.StoreItemDescriptor{Version: 3, Item: nil})
	value, detail, err := client.JSONVariationDetail(flagKey, user, ldvalue.Null())
	assert.Error(t, err)
	assert.Equal(t, ldvalue.Null(), value)
	assert.Equal(t, ldreason.EvalErrorFlagNotFound, detail.Reason.GetErrorKind())

	assert.Len(t, mockLog.GetOutput(ldlog.Error), 0)
}

func TestClientWithBuiltInPersistentStores(t *testing.T) {
	if sharedtest.ShouldSkipDatabaseTests() {
		t.Skip()
		return
	}
	t.Run("Consul", func(t *testing.T) {
		testClientWithPersistentStore(t, ldconsul.DataStore())
	})
	t.Run("DynamoDB", func(t *testing.T) {
		require.NoError(t, dynamodbtest.CreateTableIfNecessary())
		testClientWithPersistentStore(t,
			lddynamodb.DataStore(dynamodbtest.TestTableName).SessionOptions(dynamodbtest.MakeTestOptions()))
	})
	t.Run("Redis", func(t *testing.T) {
		testClientWithPersistentStore(t, ldredis.DataStore())
	})
}

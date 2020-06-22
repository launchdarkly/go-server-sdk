package ldclient

import (
	"testing"
	"time"

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
	flag := singleValueFlag("flagkey", ldvalue.String("a"))
	data := []interfaces.StoreCollection{
		{Kind: interfaces.DataKindFeatures(), Items: []interfaces.StoreKeyedItemDescriptor{
			{Key: flag.Key, Item: sharedtest.FlagDescriptor(flag)},
		}},
		{Kind: interfaces.DataKindSegments(), Items: nil},
	}
	dataSourceFactory := &dataSourceFactoryThatExposesUpdater{ // allows us to simulate an update
		underlyingFactory: dataSourceFactoryWithData{data: data},
	}
	user := lduser.NewUser("userkey")
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

	// verify that the client can get the flag from the store
	value, err := client.StringVariation(flag.Key, user, "")
	assert.NoError(t, err)
	assert.Equal(t, "a", value)

	// verify that the client can get all flags
	state := client.AllFlagsState(user)
	assert.Equal(t, map[string]ldvalue.Value{flag.Key: ldvalue.String("a")}, state.ToValuesMap())

	// verify that an update is persisted
	flagB := singleValueFlag(flag.Key, ldvalue.String("b"))
	flagB.Version = 2
	dataSourceFactory.dataSourceUpdates.Upsert(interfaces.DataKindFeatures(), flag.Key,
		sharedtest.FlagDescriptor(flagB))
	value, err = client.StringVariation(flag.Key, user, "")
	assert.NoError(t, err)
	assert.Equal(t, "b", value)

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

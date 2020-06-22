package lddynamodb

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest/dynamodbtest"
)

func TestDynamoDBDataStore(t *testing.T) {
	if !sharedtest.ShouldSkipDatabaseTests() {
		err := dynamodbtest.CreateTableIfNecessary()
		require.NoError(t, err)
	}

	sharedtest.NewPersistentDataStoreTestSuite(makeTestStore, clearTestData).
		ErrorStoreFactory(makeFailedStore(), verifyFailedStoreError).
		ConcurrentModificationHook(setConcurrentModificationHook).
		Run(t)
}

func baseBuilder() *DataStoreBuilder {
	return DataStore(dynamodbtest.TestTableName).SessionOptions(dynamodbtest.MakeTestOptions())
}

func makeTestStore(prefix string) interfaces.PersistentDataStoreFactory {
	return baseBuilder().Prefix(prefix)
}

func makeFailedStore() interfaces.PersistentDataStoreFactory {
	// Here we ensure that all DynamoDB operations will fail by simply *not* using makeTestOptions(),
	// so that the client does not have the necessary region parameter.
	return DataStore(dynamodbtest.TestTableName)
}

func verifyFailedStoreError(t *testing.T, err error) {
	assert.Contains(t, err.Error(), "could not find region configuration")
}

func clearTestData(prefix string) error {
	if prefix != "" {
		prefix += ":"
	}

	client, err := dynamodbtest.CreateTestClient()
	if err != nil {
		return err
	}
	var items []map[string]*dynamodb.AttributeValue

	err = client.ScanPages(&dynamodb.ScanInput{
		TableName:            aws.String(dynamodbtest.TestTableName),
		ConsistentRead:       aws.Bool(true),
		ProjectionExpression: aws.String("#namespace, #key"),
		ExpressionAttributeNames: map[string]*string{
			"#namespace": aws.String(tablePartitionKey),
			"#key":       aws.String(tableSortKey),
		},
	}, func(out *dynamodb.ScanOutput, lastPage bool) bool {
		items = append(items, out.Items...)
		return !lastPage
	})
	if err != nil {
		return err
	}

	var requests []*dynamodb.WriteRequest
	for _, item := range items {
		if strings.HasPrefix(*item[tablePartitionKey].S, prefix) {
			requests = append(requests, &dynamodb.WriteRequest{
				DeleteRequest: &dynamodb.DeleteRequest{Key: item},
			})
		}
	}
	return batchWriteRequests(client, dynamodbtest.TestTableName, requests)
}

func setConcurrentModificationHook(store interfaces.PersistentDataStore, hook func()) {
	store.(*dynamoDBDataStore).testUpdateHook = hook
}

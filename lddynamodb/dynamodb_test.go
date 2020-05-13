package lddynamodb

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

const (
	localDynamoEndpoint = "http://localhost:8000"
	testTableName       = "LD_DYNAMODB_TEST_TABLE"
)

func TestDynamoDBDataStore(t *testing.T) {
	err := createTableIfNecessary()
	require.NoError(t, err)

	sharedtest.NewPersistentDataStoreTestSuite(makeTestStore, clearTestData).
		ConcurrentModificationHook(setConcurrentModificationHook).
		Run(t)
}

func baseBuilder() *DynamoDBDataStoreBuilder {
	return DataStore(testTableName).SessionOptions(makeTestOptions())
}

func makeTestStore(prefix string) interfaces.PersistentDataStoreFactory {
	return baseBuilder().Prefix(prefix)
}

func clearTestData(prefix string) error {
	if prefix == "" {
		prefix = DefaultPrefix
	}

	client, err := createTestClient()
	if err != nil {
		return err
	}
	var items []map[string]*dynamodb.AttributeValue

	err = client.ScanPages(&dynamodb.ScanInput{
		TableName:            aws.String(testTableName),
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
		if strings.HasPrefix(*item[tablePartitionKey].S, prefix+":") {
			requests = append(requests, &dynamodb.WriteRequest{
				DeleteRequest: &dynamodb.DeleteRequest{Key: item},
			})
		}
	}
	return batchWriteRequests(client, testTableName, requests)
}

func setConcurrentModificationHook(store interfaces.PersistentDataStore, hook func()) {
	store.(*dynamoDBDataStore).testUpdateHook = hook
}

func makeTestOptions() session.Options {
	return session.Options{
		Config: aws.Config{
			Credentials: credentials.NewStaticCredentials("dummy", "not", "used"),
			Endpoint:    aws.String(localDynamoEndpoint),
			Region:      aws.String("us-east-1"), // this is ignored for a local instance, but is still required
		},
	}
}

func createTestClient() (*dynamodb.DynamoDB, error) {
	sess, err := session.NewSessionWithOptions(makeTestOptions())
	if err != nil {
		return nil, err
	}
	return dynamodb.New(sess), nil
}

func createTableIfNecessary() error {
	client, err := createTestClient()
	if err != nil {
		return err
	}
	_, err = client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(testTableName)})
	if err == nil {
		return nil
	}
	if e, ok := err.(awserr.Error); !ok || e.Code() != dynamodb.ErrCodeResourceNotFoundException {
		return err
	}
	createParams := dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			&dynamodb.AttributeDefinition{
				AttributeName: aws.String(tablePartitionKey),
				AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
			},
			&dynamodb.AttributeDefinition{
				AttributeName: aws.String(tableSortKey),
				AttributeType: aws.String(dynamodb.ScalarAttributeTypeS),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			&dynamodb.KeySchemaElement{
				AttributeName: aws.String(tablePartitionKey),
				KeyType:       aws.String(dynamodb.KeyTypeHash),
			},
			&dynamodb.KeySchemaElement{
				AttributeName: aws.String(tableSortKey),
				KeyType:       aws.String(dynamodb.KeyTypeRange),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		TableName: aws.String(testTableName),
	}
	_, err = client.CreateTable(&createParams)
	if err != nil {
		return err
	}
	// When DynamoDB creates a table, it may not be ready to use immediately
	deadline := time.After(10 * time.Second)
	retry := time.Tick(100 * time.Millisecond)
	for {
		select {
		case <-deadline:
			return fmt.Errorf("Timed out waiting for new table to be ready")
		case <-retry:
			tableInfo, err := client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(testTableName)})
			if err == nil && *tableInfo.Table.TableStatus == dynamodb.TableStatusActive {
				return nil
			}
		}
	}
}

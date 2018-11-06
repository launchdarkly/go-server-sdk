package lddynamodb

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

const (
	localDynamoEndpoint = "http://localhost:8000"
	testTableName       = "LD_DYNAMODB_TEST_TABLE"
)

func TestDynamoDBFeatureStore(t *testing.T) {
	err := createTableIfNecessary(testTableName)
	require.NoError(t, err)

	ldtest.RunFeatureStoreTests(t, func() ld.FeatureStore {
		store, err := NewDynamoDBFeatureStore(testTableName, SessionOptions(makeTestOptions()))
		require.NoError(t, err)
		return store
	})
}

func makeTestOptions() session.Options {
	return session.Options{
		Config: aws.Config{
			Endpoint: aws.String(localDynamoEndpoint),
			Region:   aws.String("us-east-1"),
		},
	}
}

func createTableIfNecessary(table string) error {
	sess, err := session.NewSessionWithOptions(makeTestOptions())
	if err != nil {
		return err
	}
	client := dynamodb.New(sess)
	_, err = client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(table)})
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
		TableName: aws.String(table),
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
			tableInfo, err := client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(table)})
			if err == nil && *tableInfo.Table.TableStatus == dynamodb.TableStatusActive {
				return nil
			}
		}
	}
}

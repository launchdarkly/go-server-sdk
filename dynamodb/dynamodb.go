/*
Package dynamodb provides a DynamoDB-backed feature store for the LaunchDarkly
Go SDK.

By caching feature flag data in DynamoDB, LaunchDarkly clients don't need to
call out to the LaunchDarkly API every time they're created. This is useful for
environments like AWS Lambda where workloads can be sensitive to cold starts.

In contrast to the Redis-backed feature store, the DynamoDB store can be used
without requiring access to any VPC resources, i.e. ElastiCache Redis. See
https://blog.launchdarkly.com/go-serveless-not-flagless-implementing-feature-flags-in-serverless-environments/
for more background information.

Here's how to use the feature store with the LaunchDarkly client:

	store, err := dynamodb.NewDynamoDBFeatureStore("some-table", nil)
	if err != nil { ... }

	config := ld.DefaultConfig
	config.FeatureStore = store
	config.UseLdd = true // Enable daemon mode to only read flags from DynamoDB

	ldClient, err := ld.MakeCustomClient("some-sdk-key", config, 5*time.Second)
	if err != nil { ... }
*/
package dynamodb

// This is based on code from https://github.com/mlafeldt/launchdarkly-dynamo-store

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	ld "gopkg.in/launchdarkly/go-client.v4"
)

const (
	// Schema of the DynamoDB table
	tablePartitionKey = "namespace"
	tableSortKey      = "key"
)

// Verify that the store satisfies the FeatureStore interface
var _ ld.FeatureStore = (*DBFeatureStore)(nil)

// DBFeatureStore provides a DynamoDB-backed feature store for LaunchDarkly.
type DBFeatureStore struct {
	// Client to access DynamoDB
	Client dynamodbiface.DynamoDBAPI

	// Name of the DynamoDB table
	Table string

	// Logger to write all log messages to
	Logger ld.Logger

	initialized bool
}

// NewDynamoDBFeatureStore creates a new DynamoDB feature store ready to be used
// by the LaunchDarkly client.
//
// This function uses https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#NewSession
// to configure access to DynamoDB, which means that environment variables like
// AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and AWS_REGION work as expected.
//
// For more control, compose your own DynamoDBFeatureStore with a custom DynamoDB client.
func NewDynamoDBFeatureStore(table string, config *aws.Config, logger ld.Logger) (*DBFeatureStore, error) {
	if logger == nil {
		logger = log.New(os.Stderr, "[LaunchDarkly DynamoDBFeatureStore]", log.LstdFlags)
	}

	sess, err := session.NewSession(config)
	if err != nil {
		return nil, err
	}
	client := dynamodb.New(sess)

	return &DBFeatureStore{
		Client:      client,
		Table:       table,
		Logger:      logger,
		initialized: false,
	}, nil
}

// Init initializes the store by writing the given data to DynamoDB. It will
// delete all existing data from the table.
func (store *DBFeatureStore) Init(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
	// FIXME: deleting all items before storing new ones is racy, or isn't it?
	if err := store.truncateTable(); err != nil {
		store.Logger.Printf("ERROR: Failed to truncate table: %s", err)
		return err
	}

	var requests []*dynamodb.WriteRequest

	for kind, items := range allData {
		for k, v := range items {
			av, err := marshalItem(kind, v)
			if err != nil {
				store.Logger.Printf("ERROR: Failed to marshal item (key=%s): %s", k, err)
				return err
			}
			requests = append(requests, &dynamodb.WriteRequest{
				PutRequest: &dynamodb.PutRequest{Item: av},
			})
		}
	}

	if err := store.batchWriteRequests(requests); err != nil {
		store.Logger.Printf("ERROR: Failed to write %d item(s) in batches: %s", len(requests), err)
		return err
	}

	store.Logger.Printf("INFO: Initialized table %q with %d item(s)", store.Table, len(requests))

	store.initialized = true

	return nil
}

// Initialized returns true if the store has been initialized.
func (store *DBFeatureStore) Initialized() bool {
	return store.initialized
}

// All returns all items currently stored in DynamoDB that are of the given
// data kind. (It won't return items marked as deleted.)
func (store *DBFeatureStore) All(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	var items []map[string]*dynamodb.AttributeValue

	err := store.Client.QueryPages(&dynamodb.QueryInput{
		TableName:      aws.String(store.Table),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			tablePartitionKey: {
				ComparisonOperator: aws.String("EQ"),
				AttributeValueList: []*dynamodb.AttributeValue{
					{S: aws.String(kind.GetNamespace())},
				},
			},
		},
	}, func(out *dynamodb.QueryOutput, lastPage bool) bool {
		items = append(items, out.Items...)
		return !lastPage
	})
	if err != nil {
		store.Logger.Printf("ERROR: Failed to get all %q items: %s", kind.GetNamespace(), err)
		return nil, err
	}

	results := make(map[string]ld.VersionedData)

	for _, i := range items {
		item, err := unmarshalItem(kind, i)
		if err != nil {
			store.Logger.Printf("ERROR: Failed to unmarshal item: %s", err)
			return nil, err
		}
		if !item.IsDeleted() {
			results[item.GetKey()] = item
		}
	}

	return results, nil
}

// Get returns a specific item with the given key. It returns nil if the item
// does not exist or if it's marked as deleted.
func (store *DBFeatureStore) Get(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	result, err := store.Client.GetItem(&dynamodb.GetItemInput{
		TableName:      aws.String(store.Table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(kind.GetNamespace())},
			tableSortKey:      {S: aws.String(key)},
		},
	})
	if err != nil {
		store.Logger.Printf("ERROR: Failed to get item (key=%s): %s", key, err)
		return nil, err
	}

	if len(result.Item) == 0 {
		store.Logger.Printf("DEBUG: Item not found (key=%s)", key)
		return nil, nil
	}

	item, err := unmarshalItem(kind, result.Item)
	if err != nil {
		store.Logger.Printf("ERROR: Failed to unmarshal item (key=%s): %s", key, err)
		return nil, err
	}

	if item.IsDeleted() {
		store.Logger.Printf("DEBUG: Attempted to get deleted item (key=%s)", key)
		return nil, nil
	}

	return item, nil
}

// Upsert either creates a new item of the given data kind if it doesn't
// already exist, or updates an existing item if the given item has a higher
// version.
func (store *DBFeatureStore) Upsert(kind ld.VersionedDataKind, item ld.VersionedData) error {
	return store.updateWithVersioning(kind, item)
}

// Delete marks an item as deleted. (It won't actually remove the item from
// DynamoDB.)
func (store *DBFeatureStore) Delete(kind ld.VersionedDataKind, key string, version int) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return store.updateWithVersioning(kind, deletedItem)
}

func (store *DBFeatureStore) updateWithVersioning(kind ld.VersionedDataKind, item ld.VersionedData) error {
	av, err := marshalItem(kind, item)
	if err != nil {
		store.Logger.Printf("ERROR: Failed to marshal item (key=%s): %s", item.GetKey(), err)
		return err
	}

	_, err = store.Client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(store.Table),
		Item:      av,
		ConditionExpression: aws.String(
			"attribute_not_exists(#namespace) or " +
				"attribute_not_exists(#key) or " +
				":version > #version",
		),
		ExpressionAttributeNames: map[string]*string{
			"#namespace": aws.String(tablePartitionKey),
			"#key":       aws.String(tableSortKey),
			"#version":   aws.String("version"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":version": &dynamodb.AttributeValue{N: aws.String(strconv.Itoa(item.GetVersion()))},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
			store.Logger.Printf("DEBUG: Not updating item due to condition (key=%s version=%d)",
				item.GetKey(), item.GetVersion())
			return nil
		}
		store.Logger.Printf("ERROR: Failed to put item (key=%s): %s", item.GetKey(), err)
		return err
	}

	return nil
}

// truncateTable deletes all items from the table.
func (store *DBFeatureStore) truncateTable() error {
	var items []map[string]*dynamodb.AttributeValue

	err := store.Client.ScanPages(&dynamodb.ScanInput{
		TableName:            aws.String(store.Table),
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
		store.Logger.Printf("ERROR: Failed to get all items: %s", err)
		return err
	}

	requests := make([]*dynamodb.WriteRequest, 0, len(items))

	for _, item := range items {
		requests = append(requests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{Key: item},
		})
	}

	if err := store.batchWriteRequests(requests); err != nil {
		store.Logger.Printf("ERROR: Failed to delete %d item(s) in batches: %s", len(items), err)
		return err
	}

	return nil
}

// batchWriteRequests executes a list of write requests (PutItem or DeleteItem)
// in batches of 25, which is the maximum BatchWriteItem can handle.
func (store *DBFeatureStore) batchWriteRequests(requests []*dynamodb.WriteRequest) error {
	for len(requests) > 0 {
		batchSize := int(math.Min(float64(len(requests)), 25))
		batch := requests[:batchSize]
		requests = requests[batchSize:]

		_, err := store.Client.BatchWriteItem(&dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]*dynamodb.WriteRequest{store.Table: batch},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func marshalItem(kind ld.VersionedDataKind, item ld.VersionedData) (map[string]*dynamodb.AttributeValue, error) {
	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		return nil, err
	}

	// Adding the namespace as a partition key allows us to store everything
	// (feature flags, segments, etc.) in a single DynamoDB table. The
	// namespace attribute will be ignored when unmarshalling.
	av[tablePartitionKey] = &dynamodb.AttributeValue{S: aws.String(kind.GetNamespace())}

	return av, nil
}

func unmarshalItem(kind ld.VersionedDataKind, item map[string]*dynamodb.AttributeValue) (ld.VersionedData, error) {
	data := kind.GetDefaultItem()
	if err := dynamodbattribute.UnmarshalMap(item, &data); err != nil {
		return nil, err
	}
	if item, ok := data.(ld.VersionedData); ok {
		return item, nil
	}
	return nil, fmt.Errorf("Unexpected data type from unmarshal: %T", data)
}

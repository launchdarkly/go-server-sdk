/*
Package lddynamodb provides a DynamoDB-backed feature store for the LaunchDarkly
Go SDK.

By caching feature flag data in DynamoDB, LaunchDarkly clients don't need to
call out to the LaunchDarkly API every time they're created. This is useful for
environments like AWS Lambda where workloads can be sensitive to cold starts.

In contrast to the Redis-backed feature store, the DynamoDB store can be used
without requiring access to any VPC resources, i.e. ElastiCache Redis. See
https://launchdarkly.com/blog/go-serveless-not-flagless-implementing-feature-flags-in-serverless-environments/
for more background information.

Here's how to use the feature store with the LaunchDarkly client:

	store, err := lddynamodb.NewDynamoDBFeatureStore("some-table")
	if err != nil { ... }

	config := ld.DefaultConfig
	config.FeatureStore = store
	config.UseLdd = true // Enable daemon mode to only read flags from DynamoDB

	ldClient, err := ld.MakeCustomClient("some-sdk-key", config, 5*time.Second)
	if err != nil { ... }
*/
package lddynamodb

// This is based on code from https://github.com/mlafeldt/launchdarkly-dynamo-store.
// Changes include a different method of configuration, less potential for race conditions,
// and unit tests that run against a local Dynamo instance.

// Implementation notes:
//
// - Feature flags, segments, and any other kind of entity the LaunchDarkly client may wish
// to store, are all put in the same table. The only two required attributes are "key" (which
// is present in all storeable entities) and "namespace" (a parameter from the client that is
// used to disambiguate between flags and segments; this is stored in the marshaled entity
// but is ignored during unmarshaling).
//
// - Since DynamoDB doesn't have transactions, the Init method - which replaces the entire data
// store - is not atomic, so there can be a race condition if another process is adding new data
// via Upsert. To minimize this, we don't delete all the data at the start; instead, we update
// the items we've received, and then delete all other items. That could potentially result in
// deleting new data from another process, but that would be the case anyway if the Init
// happened to execute later than the Upsert; we are relying on the fact that normally the
// process that did the Init will also receive the new data shortly and do its own Upsert.

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	ld "gopkg.in/launchdarkly/go-client.v4"
	"gopkg.in/launchdarkly/go-client.v4/utils"
)

const (
	// DefaultCacheTTL is the amount of time that recently read or updated items will be cached
	// in memory, unless you specify otherwise with the CacheTTL option.
	DefaultCacheTTL = 15 * time.Second
)

const (
	// Schema of the DynamoDB table
	tablePartitionKey = "namespace"
	tableSortKey      = "key"
	initedKey         = "$inited"
)

type namespaceAndKey struct {
	namespace string
	key       string
}

// Internal type for our DynamoDB implementation of the ld.FeatureStore interface.
type dynamoDBFeatureStore struct {
	client         dynamodbiface.DynamoDBAPI
	table          string
	cacheTTL       time.Duration
	sessionOptions session.Options
	logger         ld.Logger
	testUpdateHook func() // Used only by unit tests - see updateWithVersioning
}

// FeatureStoreOption is the interface for optional configuration parameters that can be
// passed to NewDynamoDBFeatureStore. These include SessionOptions, DynamoClient, and Logger.
type FeatureStoreOption interface {
	apply(store *dynamoDBFeatureStore) error
}

type cacheTTLOption struct {
	cacheTTL time.Duration
}

func (o cacheTTLOption) apply(store *dynamoDBFeatureStore) error {
	store.cacheTTL = o.cacheTTL
	return nil
}

// CacheTTL sets the amount of time that recently read or updated items should remain in an
// in-memory cache. This reduces the amount of database access if the same feature flags
// are being evaluated repeatedly. If it is zero, there will be no in-memory caching. The
// default value is DefaultCacheTTL.
func CacheTTL(ttl time.Duration) FeatureStoreOption {
	return cacheTTLOption{ttl}
}

type dynamoClientOption struct {
	client dynamodbiface.DynamoDBAPI
}

func (o dynamoClientOption) apply(store *dynamoDBFeatureStore) error {
	store.client = o.client
	return nil
}

// DynamoClient creates an option for NewDynamoDBFeatureStore, to specify an existing
// DynamoDB client instance. Use this if you want to customize the client used by the
// feature store in ways that are not supported by other NewDynamoDBFeatureStore options.
// If you specify this option, then any configuration specified with SessionOptions will
// be ignored.
func DynamoClient(client dynamodbiface.DynamoDBAPI) FeatureStoreOption {
	return dynamoClientOption{client}
}

type sessionOptionsOption struct {
	options session.Options
}

func (o sessionOptionsOption) apply(store *dynamoDBFeatureStore) error {
	store.sessionOptions = o.options
	return nil
}

// SessionOptions creates an option for NewDynamoDBFeatureStore, to specify an AWS
// Session.Options object to use when creating the DynamoDB session. This can be used to
// set properties such as the region programmatically, rather than relying on the
// defaults from the environment.
func SessionOptions(options session.Options) FeatureStoreOption {
	return sessionOptionsOption{options}
}

type loggerOption struct {
	logger ld.Logger
}

func (o loggerOption) apply(store *dynamoDBFeatureStore) error {
	store.logger = o.logger
	return nil
}

// Logger creates an option for NewDynamoDBFeatureStore, to specify where to send log output.
// If not specified, a log.Logger is used.
func Logger(logger ld.Logger) FeatureStoreOption {
	return loggerOption{logger}
}

// NewDynamoDBFeatureStore creates a new DynamoDB feature store to be used by the LaunchDarkly client.
//
// By default, this function uses https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#NewSession
// to configure access to DynamoDB, so the configuration will use your local AWS credentials as well
// as AWS environment variables. You can also override the default configuration with the SessionOptions
// option, or use an already-configured DynamoDB client instance with the DynamoClient option.
//
// For other options that can be customized, see CacheTTL and Logger.
func NewDynamoDBFeatureStore(table string, options ...FeatureStoreOption) (ld.FeatureStore, error) {
	store, err := newDynamoDBFeatureStoreInternal(table, options...)
	if err != nil {
		return nil, err
	}
	return utils.NewFeatureStoreWrapper(store), nil
}

func newDynamoDBFeatureStoreInternal(table string, options ...FeatureStoreOption) (*dynamoDBFeatureStore, error) {
	store := dynamoDBFeatureStore{
		table:    table,
		cacheTTL: DefaultCacheTTL,
	}

	for _, o := range options {
		err := o.apply(&store)
		if err != nil {
			return nil, err
		}
	}

	if store.logger == nil {
		store.logger = log.New(os.Stderr, "[LaunchDarkly DynamoDBFeatureStore]", log.LstdFlags)
	}

	if store.client == nil {
		sess, err := session.NewSessionWithOptions(store.sessionOptions)
		if err != nil {
			return nil, err
		}
		store.client = dynamodb.New(sess)
	}

	return &store, nil
}

func (store *dynamoDBFeatureStore) GetCacheTTL() time.Duration {
	return store.cacheTTL
}

func (store *dynamoDBFeatureStore) InitInternal(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
	// Start by reading the existing keys; we will later delete any of these that weren't in allData.
	unusedOldKeys, err := store.readExistingKeys()
	if err != nil {
		store.logger.Printf("ERROR: Failed to get existing items prior to Init: %s", err)
		return err
	}

	requests := make([]*dynamodb.WriteRequest, 0)
	numItems := 0

	// Insert or update every provided item
	for kind, items := range allData {
		for k, v := range items {
			av, err := marshalItem(kind, v)
			if err != nil {
				store.logger.Printf("ERROR: Failed to marshal item (key=%s): %s", k, err)
				return err
			}
			requests = append(requests, &dynamodb.WriteRequest{
				PutRequest: &dynamodb.PutRequest{Item: av},
			})
			nk := namespaceAndKey{namespace: kind.GetNamespace(), key: v.GetKey()}
			unusedOldKeys[nk] = false
			numItems++
		}
	}

	// Now delete any previously existing items whose keys were not in the current data
	for k, v := range unusedOldKeys {
		if v && k.namespace != initedKey {
			delKey := map[string]*dynamodb.AttributeValue{
				tablePartitionKey: &dynamodb.AttributeValue{S: aws.String(k.namespace)},
				tableSortKey:      &dynamodb.AttributeValue{S: aws.String(k.key)},
			}
			requests = append(requests, &dynamodb.WriteRequest{
				DeleteRequest: &dynamodb.DeleteRequest{Key: delKey},
			})
		}
	}

	// Now set the special key that we check in InitializedInternal()
	initedItem := map[string]*dynamodb.AttributeValue{
		tablePartitionKey: &dynamodb.AttributeValue{S: aws.String(initedKey)},
		tableSortKey:      &dynamodb.AttributeValue{S: aws.String(initedKey)},
	}
	requests = append(requests, &dynamodb.WriteRequest{
		PutRequest: &dynamodb.PutRequest{Item: initedItem},
	})

	if err := batchWriteRequests(store.client, store.table, requests); err != nil {
		store.logger.Printf("ERROR: Failed to write %d item(s) in batches: %s", len(requests), err)
		return err
	}

	store.logger.Printf("INFO: Initialized table %q with %d item(s)", store.table, numItems)

	return nil
}

func (store *dynamoDBFeatureStore) InitializedInternal() bool {
	result, err := store.client.GetItem(&dynamodb.GetItemInput{
		TableName:      aws.String(store.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(initedKey)},
			tableSortKey:      {S: aws.String(initedKey)},
		},
	})
	return err == nil && len(result.Item) != 0
}

func (store *dynamoDBFeatureStore) GetAllInternal(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	var items []map[string]*dynamodb.AttributeValue

	err := store.client.QueryPages(&dynamodb.QueryInput{
		TableName:      aws.String(store.table),
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
		store.logger.Printf("ERROR: Failed to get all %q items: %s", kind.GetNamespace(), err)
		return nil, err
	}

	results := make(map[string]ld.VersionedData)

	for _, i := range items {
		item, err := unmarshalItem(kind, i)
		if err != nil {
			store.logger.Printf("ERROR: Failed to unmarshal item: %s", err)
			return nil, err
		}
		if !item.IsDeleted() {
			results[item.GetKey()] = item
		}
	}

	return results, nil
}

func (store *dynamoDBFeatureStore) GetInternal(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	result, err := store.client.GetItem(&dynamodb.GetItemInput{
		TableName:      aws.String(store.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(kind.GetNamespace())},
			tableSortKey:      {S: aws.String(key)},
		},
	})
	if err != nil {
		store.logger.Printf("ERROR: Failed to get item (key=%s): %s", key, err)
		return nil, err
	}

	if len(result.Item) == 0 {
		store.logger.Printf("DEBUG: Item not found (key=%s)", key)
		return nil, nil
	}

	item, err := unmarshalItem(kind, result.Item)
	if err != nil {
		store.logger.Printf("ERROR: Failed to unmarshal item (key=%s): %s", key, err)
		return nil, err
	}

	return item, nil
}

func (store *dynamoDBFeatureStore) UpsertInternal(kind ld.VersionedDataKind, item ld.VersionedData) (bool, error) {
	av, err := marshalItem(kind, item)
	if err != nil {
		store.logger.Printf("ERROR: Failed to marshal item (key=%s): %s", item.GetKey(), err)
		return false, err
	}

	if store.testUpdateHook != nil {
		store.testUpdateHook()
	}

	_, err = store.client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(store.table),
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
			store.logger.Printf("DEBUG: Not updating item due to condition (namespace=%s key=%s version=%d)",
				kind.GetNamespace(), item.GetKey(), item.GetVersion())
			return false, nil
		}
		store.logger.Printf("ERROR: Failed to put item (namespace=%s key=%s): %s", kind.GetNamespace(), item.GetKey(), err)
		return false, err
	}

	return true, nil
}

func (store *dynamoDBFeatureStore) readExistingKeys() (map[namespaceAndKey]bool, error) {
	keys := make(map[namespaceAndKey]bool)
	err := store.client.ScanPages(&dynamodb.ScanInput{
		TableName:            aws.String(store.table),
		ConsistentRead:       aws.Bool(true),
		ProjectionExpression: aws.String("#namespace, #key"),
		ExpressionAttributeNames: map[string]*string{
			"#namespace": aws.String(tablePartitionKey),
			"#key":       aws.String(tableSortKey),
		},
	}, func(out *dynamodb.ScanOutput, lastPage bool) bool {
		for _, i := range out.Items {
			nk := namespaceAndKey{namespace: *(*i[tablePartitionKey]).S, key: *(*i[tableSortKey]).S}
			keys[nk] = true
		}
		return !lastPage
	})
	return keys, err
}

// batchWriteRequests executes a list of write requests (PutItem or DeleteItem)
// in batches of 25, which is the maximum BatchWriteItem can handle.
func batchWriteRequests(client dynamodbiface.DynamoDBAPI, table string, requests []*dynamodb.WriteRequest) error {
	for len(requests) > 0 {
		batchSize := int(math.Min(float64(len(requests)), 25))
		batch := requests[:batchSize]
		requests = requests[batchSize:]

		_, err := client.BatchWriteItem(&dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]*dynamodb.WriteRequest{table: batch},
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

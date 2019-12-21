// Package lddynamodb provides a DynamoDB-backed data store for the LaunchDarkly Go SDK.
//
// For more details about how and why you can use a persistent data store, see:
// https://docs.launchdarkly.com/v2.0/docs/using-a-persistent-feature-store
//
// To use the DynamoDB data store with the LaunchDarkly client:
//
//     factory, err := lddynamodb.NewDynamoDBDataStoreFactory("my-table-name")
//     if err != nil { ... }
//
//     config := ld.DefaultConfig
//     config.DataStoreFactory = factory
//     client, err := ld.MakeCustomClient("sdk-key", config, 5*time.Second)
//
// Note that the specified table must already exist in DynamoDB. It must have a
// partition key of "namespace", and a sort key of "key".
//
// By default, the data store uses a basic DynamoDB client configuration that is
// equivalent to doing this:
//
//     dynamoClient := dynamodb.New(session.NewSession())
//
// This default configuration will only work if your AWS credentials and region are
// available from AWS environment variables and/or configuration files. If you want to
// set those programmatically or modify any other configuration settings, you can use
// the SessionOptions function, or use an already-configured client via the DynamoClient
// function.
//
// If you are using the same DynamoDB table as a data store for multiple LaunchDarkly
// environments, use the Prefix option and choose a different prefix string for each, so
// they will not interfere with each other's data.
package lddynamodb

// This is based on code from https://github.com/mlafeldt/launchdarkly-dynamo-store.
// Changes include a different method of configuration, a different method of marshaling
// objects, less potential for race conditions, and unit tests that run against a local
// Dynamo instance.

// Implementation notes:
//
// - Feature flags, segments, and any other kind of entity the LaunchDarkly client may wish
// to store, are all put in the same table. The only two required attributes are "key" (which
// is present in all storeable entities) and "namespace" (a parameter from the client that is
// used to disambiguate between flags and segments).
//
// - Because of DynamoDB's restrictions on attribute values (e.g. empty strings are not
// allowed), the standard DynamoDB marshaling mechanism with one attribute per object property
// is not used. Instead, the entire object is serialized to JSON and stored in a single
// attribute, "item". The "version" property is also stored as a separate attribute since it
// is used for updates.
//
// - Since DynamoDB doesn't have transactions, the Init method - which replaces the entire data
// store - is not atomic, so there can be a race condition if another process is adding new data
// via Upsert. To minimize this, we don't delete all the data at the start; instead, we update
// the items we've received, and then delete all other items. That could potentially result in
// deleting new data from another process, but that would be the case anyway if the Init
// happened to execute later than the Upsert; we are relying on the fact that normally the
// process that did the Init will also receive the new data shortly and do its own Upsert.
//
// - DynamoDB has a maximum item size of 400KB. Since each feature flag or user segment is
// stored as a single item, this mechanism will not work for extremely large flags or segments.

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	ld "gopkg.in/launchdarkly/go-server-sdk.v4"
	"gopkg.in/launchdarkly/go-server-sdk.v4/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v4/utils"
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
	versionAttribute  = "version"
	itemJSONAttribute = "item"
)

type namespaceAndKey struct {
	namespace string
	key       string
}

type dataStoreOptions struct {
	client         dynamodbiface.DynamoDBAPI
	table          string
	prefix         string
	cacheTTL       time.Duration
	configs        []*aws.Config
	sessionOptions session.Options
	logger         ld.Logger
}

// Internal type for our DynamoDB implementation of the ld.DataStore interface.
type dynamoDBDataStore struct {
	options        dataStoreOptions
	client         dynamodbiface.DynamoDBAPI
	loggers        ldlog.Loggers
	testUpdateHook func() // Used only by unit tests - see updateWithVersioning
}

// DataStoreOption is the interface for optional configuration parameters that can be
// passed to NewDynamoDBDataStoreFactory. These include SessionOptions, CacheTTL, DynamoClient,
// and Logger.
type DataStoreOption interface {
	apply(opts *dataStoreOptions) error
}

type prefixOption struct {
	prefix string
}

func (o prefixOption) apply(opts *dataStoreOptions) error {
	opts.prefix = o.prefix
	return nil
}

// Prefix creates an option for NewDynamoDBDataStoreFactory to specify a string
// that should be prepended to all partition keys used by the data store. A colon will be
// added to this automatically. If this is unspecified, no prefix will be used.
//
//     factory, err := lddynamodb.NewDynamoDBDataStoreFactory(lddynamodb.Prefix("ld-data"))
func Prefix(prefix string) DataStoreOption {
	return prefixOption{prefix}
}

type cacheTTLOption struct {
	cacheTTL time.Duration
}

func (o cacheTTLOption) apply(opts *dataStoreOptions) error {
	opts.cacheTTL = o.cacheTTL
	return nil
}

// CacheTTL creates an option for NewDynamoDBDataStoreFactory to set the amount of time
// that recently read or updated items should remain in an in-memory cache. This reduces the
// amount of database access if the same feature flags are being evaluated repeatedly.
//
// The default value is DefaultCacheTTL. A value of zero disables in-memory caching completely.
// A negative value means data is cached forever (i.e. it will only be read again from the
// database if the SDK is restarted). Use the "cached forever" mode with caution: it means
// that in a scenario where multiple processes are sharing the database, and the current
// process loses connectivity to LaunchDarkly while other processes are still receiving
// updates and writing them to the database, the current process will have stale data.
//
//     factory, err := lddynamodb.NewDynamoDBDataStoreFactory("my-table-name",
//         lddynamodb.CacheTTL(30*time.Second))
func CacheTTL(ttl time.Duration) DataStoreOption {
	return cacheTTLOption{ttl}
}

type clientConfigOption struct {
	config *aws.Config
}

func (o clientConfigOption) apply(opts *dataStoreOptions) error {
	opts.configs = append(opts.configs, o.config)
	return nil
}

// ClientConfig creates an option for NewDynamoDBDataStoreFactory to add an AWS configuration
// object for the DynamoDB client. This allows you to customize settings such as the
// retry behavior.
func ClientConfig(config *aws.Config) DataStoreOption {
	return clientConfigOption{config}
}

type dynamoClientOption struct {
	client dynamodbiface.DynamoDBAPI
}

func (o dynamoClientOption) apply(opts *dataStoreOptions) error {
	opts.client = o.client
	return nil
}

// DynamoClient creates an option for NewDynamoDBDataStoreFactory to specify an existing
// DynamoDB client instance. Use this if you want to customize the client used by the
// data store in ways that are not supported by other NewDynamoDBDataStoreFactory options.
// If you specify this option, then any configurations specified with SessionOptions or
// ClientConfig will be ignored.
//
//     factory, err := lddynamodb.NewDynamoDBDataStoreFactory("my-table-name",
//         lddynamodb.DynamoClient(myDBClient))
func DynamoClient(client dynamodbiface.DynamoDBAPI) DataStoreOption {
	return dynamoClientOption{client}
}

type sessionOptionsOption struct {
	options session.Options
}

func (o sessionOptionsOption) apply(opts *dataStoreOptions) error {
	opts.sessionOptions = o.options
	return nil
}

// SessionOptions creates an option for NewDynamoDBDataStoreFactory, to specify an AWS
// Session.Options object to use when creating the DynamoDB session. This can be used to
// set properties such as the region programmatically, rather than relying on the
// defaults from the environment.
//
//     factory, err := lddynamodb.NewDynamoDBDataStoreFactory("my-table-name",
//         lddynamodb.SessionOptions(myOptions))
func SessionOptions(options session.Options) DataStoreOption {
	return sessionOptionsOption{options}
}

type loggerOption struct {
	logger ld.Logger
}

func (o loggerOption) apply(opts *dataStoreOptions) error {
	opts.logger = o.logger
	return nil
}

// Logger creates an option for NewDynamoDBDataStore, to specify where to send log output.
// If not specified, a log.Logger is used.
//
// You normally do not need to specify a logger because it will use the same logging configuration as
// the SDK client.
//
//     store, err := lddynamodb.NewDynamoDBDataStore("my-table-name", lddynamodb.Logger(myLogger))
func Logger(logger ld.Logger) DataStoreOption {
	return loggerOption{logger}
}

// NewDynamoDBDataStoreFactory returns a factory function for a DynamoDB-backed data store with an
// optional memory cache. You may customize its behavior with DataStoreOption values, such as
// CacheTTL and SessionOptions.
//
// By default, this function uses https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#NewSession
// to configure access to DynamoDB, so the configuration will use your local AWS credentials as well
// as AWS environment variables. You can also override the default configuration with the SessionOptions
// option, or use an already-configured DynamoDB client instance with the DynamoClient option.
//
// Set the DataStoreFactory field in your Config to the returned value. Because this is specified
// as a factory function, the Consul client is not actually created until you create the SDK client.
// This also allows it to use the same logging configuration as the SDK, so you do not have to
// specify the Logger option separately.
func NewDynamoDBDataStoreFactory(table string, options ...DataStoreOption) (ld.DataStoreFactory, error) {
	configuredOptions, err := validateOptions(table, options...)
	if err != nil {
		return nil, err
	}
	return func(ldConfig ld.Config) (ld.DataStore, error) {
		store, err := newDynamoDBDataStoreInternal(configuredOptions, ldConfig)
		if err != nil {
			return nil, err
		}
		return utils.NewNonAtomicDataStoreWrapperWithConfig(store, ldConfig), nil
	}, nil
}

func validateOptions(table string, options ...DataStoreOption) (dataStoreOptions, error) {
	ret := dataStoreOptions{
		table:    table,
		cacheTTL: DefaultCacheTTL,
	}
	if table == "" {
		return ret, errors.New("table name is required")
	}
	for _, o := range options {
		err := o.apply(&ret)
		if err != nil {
			return ret, err
		}
	}
	return ret, nil
}

func newDynamoDBDataStoreInternal(configuredOptions dataStoreOptions, ldConfig ld.Config) (*dynamoDBDataStore, error) {
	store := dynamoDBDataStore{
		options: configuredOptions,
		client:  configuredOptions.client,
		loggers: ldConfig.Loggers, // copied by value so we can modify it
	}
	store.loggers.SetBaseLogger(configuredOptions.logger) // has no effect if it is nil
	store.loggers.SetPrefix("DynamoDBDataStore:")
	store.loggers.Infof(`Using DynamoDB table %s`, configuredOptions.table)

	if store.client == nil {
		sess, err := session.NewSessionWithOptions(configuredOptions.sessionOptions)
		if err != nil {
			return nil, fmt.Errorf("unable to configure DynamoDB client: %s", err)
		}
		store.client = dynamodb.New(sess, configuredOptions.configs...)
	}

	return &store, nil
}

func (store *dynamoDBDataStore) GetCacheTTL() time.Duration {
	return store.options.cacheTTL
}

func (store *dynamoDBDataStore) InitCollectionsInternal(allData []utils.StoreCollection) error {
	// Start by reading the existing keys; we will later delete any of these that weren't in allData.
	unusedOldKeys, err := store.readExistingKeys(allData)
	if err != nil {
		return fmt.Errorf("failed to get existing items prior to Init: %s", err)
	}

	requests := make([]*dynamodb.WriteRequest, 0)
	numItems := 0

	// Insert or update every provided item
	for _, coll := range allData {
		for _, item := range coll.Items {
			key := item.GetKey()
			av, err := store.marshalItem(coll.Kind, item)
			if err != nil {
				return fmt.Errorf("failed to marshal %s key %s: %s", coll.Kind, key, err)
			}
			requests = append(requests, &dynamodb.WriteRequest{
				PutRequest: &dynamodb.PutRequest{Item: av},
			})
			nk := namespaceAndKey{namespace: store.namespaceForKind(coll.Kind), key: key}
			unusedOldKeys[nk] = false
			numItems++
		}
	}

	// Now delete any previously existing items whose keys were not in the current data
	initedKey := store.initedKey()
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

	if err := batchWriteRequests(store.client, store.options.table, requests); err != nil {
		return fmt.Errorf("failed to write %d items(s) in batches: %s", len(requests), err)
	}

	store.loggers.Infof("Initialized table %q with %d item(s)", store.options.table, numItems)

	return nil
}

func (store *dynamoDBDataStore) InitializedInternal() bool {
	result, err := store.client.GetItem(&dynamodb.GetItemInput{
		TableName:      aws.String(store.options.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(store.initedKey())},
			tableSortKey:      {S: aws.String(store.initedKey())},
		},
	})
	return err == nil && len(result.Item) != 0
}

func (store *dynamoDBDataStore) GetAllInternal(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	var items []map[string]*dynamodb.AttributeValue

	err := store.client.QueryPages(store.makeQueryForKind(kind),
		func(out *dynamodb.QueryOutput, lastPage bool) bool {
			items = append(items, out.Items...)
			return !lastPage
		})
	if err != nil {
		return nil, err
	}

	results := make(map[string]ld.VersionedData)

	for _, i := range items {
		item, err := unmarshalItem(kind, i)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %s", kind, err)
		}
		results[item.GetKey()] = item
	}

	return results, nil
}

func (store *dynamoDBDataStore) GetInternal(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	result, err := store.client.GetItem(&dynamodb.GetItemInput{
		TableName:      aws.String(store.options.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(store.namespaceForKind(kind))},
			tableSortKey:      {S: aws.String(key)},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get %s key %s: %s", kind, key, err)
	}

	if len(result.Item) == 0 {
		store.loggers.Debugf("Item not found (key=%s)", key)
		return nil, nil
	}

	item, err := unmarshalItem(kind, result.Item)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s key %s: %s", kind, key, err)
	}

	return item, nil
}

func (store *dynamoDBDataStore) UpsertInternal(kind ld.VersionedDataKind, item ld.VersionedData) (ld.VersionedData, error) {
	av, err := store.marshalItem(kind, item)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s key %s: %s", kind, item.GetKey(), err)
	}

	if store.testUpdateHook != nil {
		store.testUpdateHook()
	}

	_, err = store.client.PutItem(&dynamodb.PutItemInput{
		TableName: aws.String(store.options.table),
		Item:      av,
		ConditionExpression: aws.String(
			"attribute_not_exists(#namespace) or " +
				"attribute_not_exists(#key) or " +
				":version > #version",
		),
		ExpressionAttributeNames: map[string]*string{
			"#namespace": aws.String(tablePartitionKey),
			"#key":       aws.String(tableSortKey),
			"#version":   aws.String(versionAttribute),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":version": &dynamodb.AttributeValue{N: aws.String(strconv.Itoa(item.GetVersion()))},
		},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == dynamodb.ErrCodeConditionalCheckFailedException {
			store.loggers.Debugf("Not updating item due to condition (namespace=%s key=%s version=%d)",
				kind.GetNamespace(), item.GetKey(), item.GetVersion())
			// We must now read the item that's in the database and return it, so DataStoreWrapper can cache it
			oldItem, err := store.GetInternal(kind, item.GetKey())
			return oldItem, err
		}
		return nil, fmt.Errorf("failed to put %s key %s: %s", kind, item.GetKey(), err)
	}

	return item, nil
}

func (store *dynamoDBDataStore) IsStoreAvailable() bool {
	// There doesn't seem to be a specific DynamoDB API for just testing the connection. We will just
	// do a simple query for the "inited" key, and test whether we get an error ("not found" does not
	// count as an error).
	_, err := store.client.GetItem(&dynamodb.GetItemInput{
		TableName:      aws.String(store.options.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(store.initedKey())},
			tableSortKey:      {S: aws.String(store.initedKey())},
		},
	})
	return err == nil
}

func (store *dynamoDBDataStore) prefixedNamespace(baseNamespace string) string {
	if store.options.prefix == "" {
		return baseNamespace
	}
	return store.options.prefix + ":" + baseNamespace
}

func (store *dynamoDBDataStore) namespaceForKind(kind ld.VersionedDataKind) string {
	return store.prefixedNamespace(kind.GetNamespace())
}

func (store *dynamoDBDataStore) initedKey() string {
	return store.prefixedNamespace("$inited")
}

func (store *dynamoDBDataStore) makeQueryForKind(kind ld.VersionedDataKind) *dynamodb.QueryInput {
	return &dynamodb.QueryInput{
		TableName:      aws.String(store.options.table),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			tablePartitionKey: {
				ComparisonOperator: aws.String("EQ"),
				AttributeValueList: []*dynamodb.AttributeValue{
					{S: aws.String(store.namespaceForKind(kind))},
				},
			},
		},
	}
}

func (store *dynamoDBDataStore) readExistingKeys(newData []utils.StoreCollection) (map[namespaceAndKey]bool, error) {
	keys := make(map[namespaceAndKey]bool)
	for _, coll := range newData {
		kind := coll.Kind
		query := store.makeQueryForKind(kind)
		query.ProjectionExpression = aws.String("#namespace, #key")
		query.ExpressionAttributeNames = map[string]*string{
			"#namespace": aws.String(tablePartitionKey),
			"#key":       aws.String(tableSortKey),
		}
		err := store.client.QueryPages(query,
			func(out *dynamodb.QueryOutput, lastPage bool) bool {
				for _, i := range out.Items {
					nk := namespaceAndKey{namespace: *(*i[tablePartitionKey]).S, key: *(*i[tableSortKey]).S}
					keys[nk] = true
				}
				return !lastPage
			})
		if err != nil {
			return nil, err
		}
	}
	return keys, nil
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

func (store *dynamoDBDataStore) marshalItem(kind ld.VersionedDataKind, item ld.VersionedData) (map[string]*dynamodb.AttributeValue, error) {
	jsonItem, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	return map[string]*dynamodb.AttributeValue{
		tablePartitionKey: &dynamodb.AttributeValue{S: aws.String(store.namespaceForKind(kind))},
		tableSortKey:      &dynamodb.AttributeValue{S: aws.String(item.GetKey())},
		versionAttribute:  &dynamodb.AttributeValue{N: aws.String(strconv.Itoa(item.GetVersion()))},
		itemJSONAttribute: &dynamodb.AttributeValue{S: aws.String(string(jsonItem))},
	}, nil
}

func unmarshalItem(kind ld.VersionedDataKind, item map[string]*dynamodb.AttributeValue) (ld.VersionedData, error) {
	if itemAttr := item[itemJSONAttribute]; itemAttr != nil && itemAttr.S != nil {
		data, err := utils.UnmarshalItem(kind, []byte(*itemAttr.S))
		return data, err
	}
	return nil, errors.New("DynamoDB map did not contain expected item string")
}

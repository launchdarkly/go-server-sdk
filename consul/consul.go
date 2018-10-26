package consul

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	c "github.com/hashicorp/consul/api"
	cache "github.com/patrickmn/go-cache"
	ld "gopkg.in/launchdarkly/go-client.v4"
)

// DefaultPrefix is a string that is prepended (along with a slash) to all Consul keys used
// by the feature store. You can change this value with the Prefix() option.
const DefaultPrefix = "launchdarkly"

// FeatureStore represents a Consul-backed feature store
type FeatureStore struct {
	config     c.Config
	prefix     string
	client     *c.Client
	cache      *cache.Cache
	timeout    time.Duration
	logger     ld.Logger
	inited     bool
	initCheck  sync.Once
	testTxHook func() // for unit testing of concurrent modifications
}

// FeatureStoreOption is the interface for optional configuration parameters that can be
// passed to NewConsulFeatureStore. These include UseConfig, Prefix, CacheTTL, and UseLogger.
type FeatureStoreOption interface {
	apply(store *FeatureStore) error
}

type configOption struct {
	config c.Config
}

func (o configOption) apply(store *FeatureStore) error {
	store.config = o.config
	return nil
}

// UseConfig creates an option for NewConsulFeatureStore, to specify an entire configuration
// for the Consul driver. This overwrites any previous Consul settings that may have been
// specified.
func UseConfig(config c.Config) FeatureStoreOption {
	return configOption{config}
}

type addressOption struct {
	address string
}

func (o addressOption) apply(store *FeatureStore) error {
	store.config.Address = o.address
	return nil
}

// Address creates an option for NewConsulFeatureStore, to set the address of the Consul server.
// If placed after ConsulConfig(), this modifies the previously specified configuration.
func Address(address string) FeatureStoreOption {
	return addressOption{address}
}

type prefixOption struct {
	prefix string
}

func (o prefixOption) apply(store *FeatureStore) error {
	store.prefix = o.prefix
	return nil
}

// Prefix creates an option for NewConsulFeatureStore, to specify a prefix for namespacing
// the feature store's keys. The default value is DefaultPrefix.
func Prefix(prefix string) FeatureStoreOption {
	return prefixOption{prefix}
}

type cacheTTLOption struct {
	ttl time.Duration
}

func (o cacheTTLOption) apply(store *FeatureStore) error {
	store.timeout = o.ttl
	return nil
}

// CacheTTL creates an option for NewConsulFeatureStore, to specify how long flag data should be
// cached in memory to avoid rereading it from Consul. If this is zero or unspecified, the feature
// store will not use an in-memory cache.
func CacheTTL(ttl time.Duration) FeatureStoreOption {
	return cacheTTLOption{ttl}
}

type loggerOption struct {
	logger ld.Logger
}

func (o loggerOption) apply(store *FeatureStore) error {
	store.logger = o.logger
	return nil
}

// UseLogger creates an option for NewConsulFeatureStore, to specify where to send log output.
// If not specified, a log.Logger is used.
func UseLogger(logger ld.Logger) FeatureStoreOption {
	return loggerOption{logger}
}

// NewConsulFeatureStore creates a new Consul-backed feature store with an optional memory cache. You
// may customize its behavior with any number of FeatureStoreOption values.
func NewConsulFeatureStore(options ...FeatureStoreOption) (*FeatureStore, error) {
	store := &FeatureStore{config: *c.DefaultConfig()}
	for _, o := range options {
		err := o.apply(store)
		if err != nil {
			return nil, err
		}
	}

	if store.logger == nil {
		store.logger = defaultLogger()
	}
	if store.prefix == "" {
		store.prefix = DefaultPrefix
	}

	store.logger.Printf("ConsulFeatureStore: Using config: %+v", store.config)

	if store.timeout > 0 {
		store.logger.Printf("ConsulFeatureStore: Using local cache with timeout: %v", store.timeout)
		store.cache = cache.New(store.timeout, 5*time.Minute)
	}

	client, err := c.NewClient(&store.config)
	if err != nil {
		return nil, err
	}
	store.client = client
	return store, nil
}

// Get returns an individual object of a given type from the store.
func (store *FeatureStore) Get(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	item, _, err := store.getEvenIfDeleted(kind, key, true)
	if err == nil && item == nil {
		store.logger.Printf("ConsulFeatureStore: WARN: Item not found in store. Key: %s", key)
	}
	if err == nil && item != nil && item.IsDeleted() {
		store.logger.Printf("ConsulFeatureStore: WARN: Attempted to get deleted item in \"%s\". Key: %s", kind.GetNamespace(), key)
		return nil, nil
	}
	return item, err
}

// All returns all the objects of a given kind from the store.
func (store *FeatureStore) All(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {

	if store.cache != nil {
		if data, present := store.cache.Get(allFlagsCacheKey(kind)); present {
			if items, ok := data.(map[string]ld.VersionedData); ok {
				return items, nil
			}
			store.logger.Printf("ERROR: ConsulFeatureStore's in-memory cache returned an unexpected type: %T. Expected map[string]ld.VersionedData", data)
		}
	}

	results := make(map[string]ld.VersionedData)

	kv := store.client.KV()
	pairs, _, err := kv.List(store.featuresKey(kind), nil)

	if err != nil {
		return results, err
	}

	for _, pair := range pairs {
		item, jsonErr := unmarshalItem(kind, pair.Value)

		if jsonErr != nil {
			return nil, err
		}

		if !item.IsDeleted() {
			results[item.GetKey()] = item
		}
	}
	if store.cache != nil {
		store.cache.Set(allFlagsCacheKey(kind), results, store.timeout)
	}
	return results, nil
}

// Init populates the store with a complete set of versioned data.
func (store *FeatureStore) Init(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {

	if store.cache != nil {
		store.cache.Flush()
	}

	kv := store.client.KV()

	ops := c.KVTxnOps{
		&c.KVTxnOp{
			Verb: c.KVDeleteTree,
			Key:  store.prefix,
		},
	}

	for kind, items := range allData {

		for k, v := range items {
			data, jsonErr := json.Marshal(v)

			if jsonErr != nil {
				return jsonErr
			}

			op := &c.KVTxnOp{
				Verb:  c.KVSet,
				Key:   store.featureKeyFor(kind, k),
				Value: data,
			}

			ops = append(ops, op)
		}

		if store.cache != nil {
			store.cache.Set(allFlagsCacheKey(kind), items, store.timeout)
		}
	}

	ok, resp, _, err := kv.Txn(ops, nil)

	if err != nil {
		return err
	}
	if !ok {
		errs := make([]string, 0)
		for _, te := range resp.Errors {
			errs = append(errs, te.What)
		}
		return fmt.Errorf("Consul transaction failed: %s", strings.Join(errs, ", "))
	}
	store.initCheck.Do(func() { store.inited = true })
	return nil
}

// Delete removes an item of a given kind from the store.
func (store *FeatureStore) Delete(kind ld.VersionedDataKind, key string, version int) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return store.updateWithVersioning(kind, deletedItem)
}

// Upsert inserts or replaces an item in the store unless there it already contains an item with an equal or larger version.
func (store *FeatureStore) Upsert(kind ld.VersionedDataKind, item ld.VersionedData) error {
	return store.updateWithVersioning(kind, item)
}

// Initialized returns true if we have already populated Consul with feature flag data.
func (store *FeatureStore) Initialized() bool {
	store.initCheck.Do(func() {
		kv := store.client.KV()
		pair, _, err := kv.Get(store.prefix, nil)
		inited := pair != nil && err == nil
		store.inited = inited
	})
	return store.inited
}

func defaultLogger() *log.Logger {
	return log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags)
}

func cacheKey(kind ld.VersionedDataKind, key string) string {
	return kind.GetNamespace() + "/" + key
}

func allFlagsCacheKey(kind ld.VersionedDataKind) string {
	return "all/" + kind.GetNamespace()
}

// TODO this should be a utility somewhere. It doesn't use the store
func unmarshalItem(kind ld.VersionedDataKind, raw []byte) (ld.VersionedData, error) {
	data := kind.GetDefaultItem()
	if jsonErr := json.Unmarshal(raw, &data); jsonErr != nil {
		return nil, jsonErr
	}
	if item, ok := data.(ld.VersionedData); ok {
		return item, nil
	}
	return nil, fmt.Errorf("unexpected data type from JSON unmarshal: %T", data)
}

func (store *FeatureStore) updateWithVersioning(kind ld.VersionedDataKind, newItem ld.VersionedData) error {
	data, jsonErr := json.Marshal(newItem)
	if jsonErr != nil {
		return jsonErr
	}
	key := newItem.GetKey()

	// We will potentially keep retrying to store indefinitely until someone's write succeeds
	for {
		// Get the item
		oldItem, modifyIndex, err := store.getEvenIfDeleted(kind, key, false)

		if err != nil {
			return err
		}

		// Check whether the item is stale. If so, just return
		if oldItem != nil && oldItem.GetVersion() >= newItem.GetVersion() {
			break
		}

		if store.testTxHook != nil { // instrumentation for unit tests
			store.testTxHook()
		}

		// Otherwise, try to write. We will do a compare-and-set operation, so the write will only succeed if
		// the key's ModifyIndex is still equal to the previous value returned by getEvenIfDeleted. If the
		// previous ModifyIndex was zero, it means the key did not previously exist and the write will only
		// succeed if it still doesn't exist.
		kv := store.client.KV()
		p := &c.KVPair{
			Key:         store.featureKeyFor(kind, key),
			ModifyIndex: modifyIndex,
			Value:       data,
		}
		written, _, err := kv.CAS(p, nil)

		if err != nil {
			return err
		}

		if written {
			// Success - clear the cache and exit
			if store.cache != nil {
				store.cache.Delete(allFlagsCacheKey(kind))
				store.cache.Set(cacheKey(kind, key), newItem, store.timeout)
			}
			break
		} else {
			// If we failed, retry the whole shebang
			store.logger.Printf("ConsulFeatureStore: DEBUG: Concurrent modification detected, retrying")
		}
	}
	return nil
}

func (store *FeatureStore) getEvenIfDeleted(kind ld.VersionedDataKind, key string,
	useCache bool) (retrievedItem ld.VersionedData, modifyIndex uint64, err error) {
	var defaultModifyIndex = uint64(0)
	if useCache && store.cache != nil {
		if data, present := store.cache.Get(cacheKey(kind, key)); present {
			item, ok := data.(ld.VersionedData)
			if ok {
				return item, defaultModifyIndex, nil
			}
			store.logger.Printf("ERROR: ConsulFeatureStore's in-memory cache returned an unexpected type: %v. Expected ld.VersionedData", reflect.TypeOf(data))
		}
	}

	kv := store.client.KV()

	pair, _, err := kv.Get(store.featureKeyFor(kind, key), nil)

	if err != nil || pair == nil {
		return nil, defaultModifyIndex, err
	}

	item, jsonErr := unmarshalItem(kind, pair.Value)

	if jsonErr != nil {
		return nil, defaultModifyIndex, jsonErr
	}

	if store.cache != nil {
		store.cache.Set(cacheKey(kind, key), item, store.timeout)
	}

	return item, pair.ModifyIndex, nil
}

func (store *FeatureStore) featuresKey(kind ld.VersionedDataKind) string {
	return store.prefix + "/" + kind.GetNamespace()
}

func (store *FeatureStore) featureKeyFor(kind ld.VersionedDataKind, k string) string {
	return store.prefix + "/" + kind.GetNamespace() + "/" + k
}

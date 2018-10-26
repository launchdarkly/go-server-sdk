package consul

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"sync"
	"time"

	consul "github.com/hashicorp/consul/api"

	cache "github.com/patrickmn/go-cache"

	ld "gopkg.in/launchdarkly/go-client.v4"
)

const (
	defaultPrefix = "launchDarkly"
)

// FeatureStore represents a Consul-backed feature store
type FeatureStore struct {
	prefix    string
	client    *consul.Client
	cache     *cache.Cache
	timeout   time.Duration
	logger    *log.Logger
	inited    bool
	initCheck sync.Once
}

// NewConsulFeatureStoreWithConfig creates a new Consul-backed feature store with an optional memory cache based on the specified Consul config.
// Attaches a prefix string to all keys to namespace LaunchDarkly-specific keys. If the
// specified prefix is the empty string, it defaults to "launchdarkly"
func NewConsulFeatureStoreWithConfig(config *consul.Config, prefix string, timeout time.Duration, logger *log.Logger) (*FeatureStore, error) {
	var c *cache.Cache
	if logger == nil {
		logger = defaultLogger()
	}
	if prefix == "" {
		prefix = defaultPrefix
	}
	logger.Printf("ConsulFeatureStore: Using config: %+v", config)

	if timeout > 0 {
		logger.Printf("ConsulFeatureStore: Using local cache with timeout: %v", timeout)
		c = cache.New(timeout, 5*time.Minute)
	}

	client, err := consul.NewClient(config)
	if err != nil {
		return nil, err
	}
	return &FeatureStore{
		prefix:  prefix,
		inited:  false,
		logger:  logger,
		timeout: timeout,
		cache:   c,
		client:  client,
	}, nil
}

// Get returns an individual object of a given type from the store
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

// All returns all the objects of a given kind from the store
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
	keyPrefix := store.featuresKey(kind)
	pairs, _, err := kv.List(keyPrefix, nil)

	if err != nil {
		return results, err
	}

	for _, pair := range pairs {
		item, jsonErr := unmarshalItem(kind, pair.Value)

		if jsonErr != nil {
			return nil, err
		}

		if !item.IsDeleted() && pair.Key[:len(keyPrefix)] == keyPrefix {
			key := pair.Key[len(keyPrefix)+1:]
			results[key] = item
		}
	}
	if store.cache != nil {
		store.cache.Set(allFlagsCacheKey(kind), results, store.timeout)
	}
	return results, nil
}

// Init populates the store with a complete set of versioned data
func (store *FeatureStore) Init(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {

	if store.cache != nil {
		store.cache.Flush()
	}

	kv := store.client.KV()

	ops := consul.KVTxnOps{
		&consul.KVTxnOp{
			Verb: consul.KVDeleteTree,
			Key:  store.prefix,
		},
	}

	for kind, items := range allData {

		for k, v := range items {
			data, jsonErr := json.Marshal(v)

			if jsonErr != nil {
				return jsonErr
			}

			op := &consul.KVTxnOp{
				Verb:  consul.KVSet,
				Key:   store.featureKeyFor(kind, k),
				Value: data,
			}

			ops = append(ops, op)
		}

		if store.cache != nil {
			store.cache.Set(allFlagsCacheKey(kind), items, store.timeout)
		}
	}

	// TODO check the response
	_, _, _, err := kv.Txn(ops, nil)

	if err != nil {
		return err
	}

	store.initCheck.Do(func() { store.inited = true })
	return nil
}

// Delete removes an item of a given kind from the store
func (store *FeatureStore) Delete(kind ld.VersionedDataKind, key string, version int) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return store.updateWithVersioning(kind, deletedItem)
}

// Upsert inserts or replaces an item in the store unless there it already contains an item with an equal or larger version
func (store *FeatureStore) Upsert(kind ld.VersionedDataKind, item ld.VersionedData) error {
	return store.updateWithVersioning(kind, item)
}

// Initialized returns whether redis contains an entry for this environment
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
			return nil
		}

		// Otherwise, try to write.
		data, jsonErr := json.Marshal(newItem)
		if jsonErr != nil {
			return jsonErr
		}

		// Compare and swap the item.
		kv := store.client.KV()

		p := &consul.KVPair{
			Key:         store.featureKeyFor(kind, key),
			ModifyIndex: modifyIndex,
			Value:       data,
		}

		written, _, err := kv.CAS(p, nil)

		if err != nil {
			return err
		}

		// If we failed, retry the whole shebang
		if !written {
			store.logger.Printf("ConsulFeatureStore: DEBUG: Concurrent modification detected, retrying")
			continue
		}

		// Otherwise, clear the cache and exit
		if store.cache != nil {
			store.cache.Delete(allFlagsCacheKey(kind))
			store.cache.Set(cacheKey(kind, key), newItem, store.timeout)
		}

		return nil
	}
}

func (store *FeatureStore) getEvenIfDeleted(kind ld.VersionedDataKind, key string, useCache bool) (ld.VersionedData, uint64, error) {
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

	if err != nil {
		return nil, defaultModifyIndex, err
	}

	if pair == nil {
		return nil, defaultModifyIndex, nil
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

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

	// TODO change this back to the gopkg dependency
	//ld "gopkg.in/launchdarkly/go-client.v4"
	ld "github.com/launchdarkly/go-client-private"
)

const (
	defaultPrefix = "launchDarkly"
)

// ConsulFeatureStore represents a Consul-backed feature store
type ConsulFeatureStore struct {
	prefix    string
	client    *consul.Client
	cache     *cache.Cache
	timeout   time.Duration
	logger    *log.Logger
	inited    bool
	initCheck sync.Once
}

// NewConsulFeatureStoreWithConfig creates a new Consul-backed feature store with an optional memory cache based on the specified Consul config
func NewConsulFeatureStoreWithConfig(config *consul.Config, prefix string, timeout time.Duration, logger *log.Logger) (*ConsulFeatureStore, error) {
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

	return &ConsulFeatureStore{
		prefix:  prefix,
		inited:  false,
		logger:  logger,
		timeout: timeout,
		cache:   c,
		client:  client,
	}, err
}

func (store *ConsulFeatureStore) featuresKey(kind ld.VersionedDataKind) string {
	return store.prefix + "/" + kind.GetNamespace()
}

func (store *ConsulFeatureStore) featureKeyFor(kind ld.VersionedDataKind, k string) string {
	return store.prefix + "/" + kind.GetNamespace() + "/" + k
}

func (store *ConsulFeatureStore) Get(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
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

func (store *ConsulFeatureStore) All(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {

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
			results[pair.Key] = item
		}
	}
	if store.cache != nil {
		store.cache.Set(allFlagsCacheKey(kind), results, store.timeout)
	}
	return results, nil
}

func (store *ConsulFeatureStore) Init(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {

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

func (store *ConsulFeatureStore) getEvenIfDeleted(kind ld.VersionedDataKind, key string, useCache bool) (ld.VersionedData, uint64, error) {
	var defaultModifyIndex = uint64(0)
	if useCache && store.cache != nil {
		if data, present := store.cache.Get(cacheKey(kind, key)); present {
			item, ok := data.(ld.VersionedData)
			if ok {
				return item, defaultModifyIndex, nil
			}
			store.logger.Printf("ERROR: RedisFeatureStore's in-memory cache returned an unexpected type: %v. Expected ld.VersionedData", reflect.TypeOf(data))
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

func (store *ConsulFeatureStore) Delete(kind ld.VersionedDataKind, key string, version int) error {
	deletedItem := kind.MakeDeletedItem(key, version)
	return store.updateWithVersioning(kind, deletedItem)
}

func (store *ConsulFeatureStore) Upsert(kind ld.VersionedDataKind, item ld.VersionedData) error {
	return store.updateWithVersioning(kind, item)
}

func (store *ConsulFeatureStore) Initialized() bool {
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

func (store *ConsulFeatureStore) updateWithVersioning(kind ld.VersionedDataKind, newItem ld.VersionedData) error {
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

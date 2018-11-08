package redis

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	r "github.com/garyburd/redigo/redis"

	ld "gopkg.in/launchdarkly/go-client.v4"
	"gopkg.in/launchdarkly/go-client.v4/utils"
)

// RedisFeatureStore is a Redis-backed feature store implementation.
type RedisFeatureStore struct { // nolint:golint // package name in type name
	wrapper *utils.FeatureStoreWrapper
	core    *redisFeatureStoreCore
}

// redisFeatureStoreCore is the internal implementation, using the simpler interface defined in
// utils.FeatureStoreCore. The FeatureStoreWrapper wraps this to add caching. The only reason that
// there is a separate RedisFeatureStore type, instead of just using the FeatureStoreWrapper itself
// as the outermost object, is a historical one: the NewRedisFeatureStore constructors had already
// been defined as returning *RedisFeatureStore rather than the interface type.
type redisFeatureStoreCore struct {
	prefix     string
	pool       *r.Pool
	cacheTTL   time.Duration
	logger     ld.Logger
	testTxHook func()
}

var pool *r.Pool

func newPool(url string) *r.Pool {
	pool = &r.Pool{
		MaxIdle:     20,
		MaxActive:   16,
		Wait:        true,
		IdleTimeout: 300 * time.Second,
		Dial: func() (c r.Conn, err error) {
			c, err = r.DialURL(url)
			return
		},
		TestOnBorrow: func(c r.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	return pool
}

const initedKey = "$inited"

// NewRedisFeatureStoreFromUrl constructs a new Redis-backed feature store connecting to the specified URL with a default
// connection pool configuration (16 concurrent connections, connection requests block).
// Attaches a prefix string to all keys to namespace LaunchDarkly-specific keys. If the
// specified prefix is the empty string, it defaults to "launchdarkly".
func NewRedisFeatureStoreFromUrl(url, prefix string, timeout time.Duration, logger ld.Logger) *RedisFeatureStore {
	if logger == nil {
		logger = defaultLogger()
	}
	logger.Printf("RedisFeatureStore: Using url: %s", url)
	return NewRedisFeatureStoreWithPool(newPool(url), prefix, timeout, logger)
}

// NewRedisFeatureStoreWithPool constructs a new Redis-backed feature store with the specified redigo pool configuration.
// Attaches a prefix string to all keys to namespace LaunchDarkly-specific keys. If the
// specified prefix is the empty string, it defaults to "launchdarkly".
func NewRedisFeatureStoreWithPool(pool *r.Pool, prefix string, timeout time.Duration, logger ld.Logger) *RedisFeatureStore {
	if logger == nil {
		logger = defaultLogger()
	}

	if prefix == "" {
		prefix = "launchdarkly"
	}
	logger.Printf("RedisFeatureStore: Using prefix: %s ", prefix)

	if timeout > 0 {
		logger.Printf("RedisFeatureStore: Using local cache with timeout: %v", timeout)
	}

	core := &redisFeatureStoreCore{
		prefix:   prefix,
		pool:     pool,
		cacheTTL: timeout,
		logger:   logger,
	}
	return &RedisFeatureStore{
		wrapper: utils.NewFeatureStoreWrapper(core),
		core:    core,
	}
}

// NewRedisFeatureStore constructs a new Redis-backed feature store connecting to the specified host and port with a default
// connection pool configuration (16 concurrent connections, connection requests block).
// Attaches a prefix string to all keys to namespace LaunchDarkly-specific keys. If the
// specified prefix is the empty string, it defaults to "launchdarkly"
func NewRedisFeatureStore(host string, port int, prefix string, timeout time.Duration, logger ld.Logger) *RedisFeatureStore {
	return NewRedisFeatureStoreFromUrl(fmt.Sprintf("redis://%s:%d", host, port), prefix, timeout, logger)
}

// Get returns an individual object of a given type from the store
func (store *RedisFeatureStore) Get(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	return store.wrapper.Get(kind, key)
}

// All returns all the objects of a given kind from the store
func (store *RedisFeatureStore) All(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	return store.wrapper.All(kind)
}

// Init populates the store with a complete set of versioned data
func (store *RedisFeatureStore) Init(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
	return store.wrapper.Init(allData)
}

// Upsert inserts or replaces an item in the store unless there it already contains an item with an equal or larger version
func (store *RedisFeatureStore) Upsert(kind ld.VersionedDataKind, item ld.VersionedData) error {
	return store.wrapper.Upsert(kind, item)
}

// Delete removes an item of a given kind from the store
func (store *RedisFeatureStore) Delete(kind ld.VersionedDataKind, key string, version int) error {
	return store.wrapper.Delete(kind, key, version)
}

// Initialized returns whether redis contains an entry for this environment
func (store *RedisFeatureStore) Initialized() bool {
	return store.wrapper.Initialized()
}

// Actual implementation methods are below - these are called by FeatureStoreWrapper, which adds
// caching behavior if necessary.

func (store *redisFeatureStoreCore) GetCacheTTL() time.Duration {
	return store.cacheTTL
}

func (store *redisFeatureStoreCore) GetInternal(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	c := store.getConn()
	defer c.Close() // nolint:errcheck

	jsonStr, err := r.String(c.Do("HGET", store.featuresKey(kind), key))

	if err != nil {
		if err == r.ErrNil {
			store.logger.Printf("RedisFeatureStore: DEBUG: Key: %s not found in \"%s\"", key, kind.GetNamespace())
			return nil, nil
		}
		return nil, err
	}

	item, jsonErr := utils.UnmarshalItem(kind, []byte(jsonStr))
	if jsonErr != nil {
		return nil, jsonErr
	}
	return item, nil
}

func (store *redisFeatureStoreCore) GetAllInternal(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	results := make(map[string]ld.VersionedData)

	c := store.getConn()
	defer c.Close() // nolint:errcheck

	values, err := r.StringMap(c.Do("HGETALL", store.featuresKey(kind)))

	if err != nil && err != r.ErrNil {
		return nil, err
	}

	for k, v := range values {
		item, jsonErr := utils.UnmarshalItem(kind, []byte(v))

		if jsonErr != nil {
			return nil, err
		}

		if !item.IsDeleted() {
			results[k] = item
		}
	}
	return results, nil
}

// Init populates the store with a complete set of versioned data
func (store *redisFeatureStoreCore) InitInternal(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
	c := store.getConn()
	defer c.Close() // nolint:errcheck

	_ = c.Send("MULTI")

	for kind, items := range allData {
		baseKey := store.featuresKey(kind)

		_ = c.Send("DEL", baseKey)

		for k, v := range items {
			data, jsonErr := json.Marshal(v)

			if jsonErr != nil {
				return jsonErr
			}

			_ = c.Send("HSET", baseKey, k, data)
		}
	}

	_ = c.Send("SET", store.initedKey(), "")

	_, err := c.Do("EXEC")

	return err
}

func (store *redisFeatureStoreCore) UpsertInternal(kind ld.VersionedDataKind, newItem ld.VersionedData) (ld.VersionedData, error) {
	baseKey := store.featuresKey(kind)
	key := newItem.GetKey()
	for {
		// We accept that we can acquire multiple connections here and defer inside loop but we don't expect many
		c := store.getConn()
		defer c.Close() // nolint:errcheck

		_, err := c.Do("WATCH", baseKey)
		if err != nil {
			return nil, err
		}

		defer c.Send("UNWATCH") // nolint:errcheck // this should always succeed

		if store.testTxHook != nil { // instrumentation for unit tests
			store.testTxHook()
		}

		oldItem, err := store.GetInternal(kind, key)

		if err != nil {
			return nil, err
		}

		if oldItem != nil && oldItem.GetVersion() >= newItem.GetVersion() {
			return oldItem, nil
		}

		data, jsonErr := json.Marshal(newItem)
		if jsonErr != nil {
			return nil, jsonErr
		}

		_ = c.Send("MULTI")
		err = c.Send("HSET", baseKey, key, data)
		if err == nil {
			var result interface{}
			result, err = c.Do("EXEC")
			if err == nil {
				if result == nil {
					// if exec returned nothing, it means the watch was triggered and we should retry
					store.logger.Printf("RedisFeatureStore: DEBUG: Concurrent modification detected, retrying")
					continue
				}
			}
			return newItem, nil
		}
		return nil, err
	}
}

func (store *redisFeatureStoreCore) InitializedInternal() bool {
	c := store.getConn()
	defer c.Close() // nolint:errcheck
	inited, _ := r.Bool(c.Do("EXISTS", store.initedKey()))
	return inited
}

func (store *redisFeatureStoreCore) featuresKey(kind ld.VersionedDataKind) string {
	return store.prefix + ":" + kind.GetNamespace()
}

func (store *redisFeatureStoreCore) initedKey() string {
	return store.prefix + ":" + initedKey
}

func (store *redisFeatureStoreCore) getConn() r.Conn {
	return store.pool.Get()
}

func defaultLogger() *log.Logger {
	return log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags)
}

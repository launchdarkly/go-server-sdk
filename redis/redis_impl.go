package redis

import (
	"encoding/json"
	"fmt"
	"time"

	r "github.com/garyburd/redigo/redis"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/utils"
)

// Internal implementation of the PersistentDataStore interface for Redis.
type redisDataStoreImpl struct {
	prefix     string
	pool       *r.Pool
	loggers    ldlog.Loggers
	testTxHook func()
}

func newPool(url string, dialOptions []r.DialOption) *r.Pool {
	pool := &r.Pool{
		MaxIdle:     20,
		MaxActive:   16,
		Wait:        true,
		IdleTimeout: 300 * time.Second,
		Dial: func() (c r.Conn, err error) {
			c, err = r.DialURL(url, dialOptions...)
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

func newRedisDataStoreImpl(
	builder *DataStoreBuilder,
	loggers ldlog.Loggers,
) *redisDataStoreImpl {
	impl := &redisDataStoreImpl{
		prefix:  builder.prefix,
		pool:    builder.pool,
		loggers: loggers,
	}
	impl.loggers.SetPrefix("RedisDataStore:")

	if impl.pool == nil {
		impl.loggers.Infof("Using url: %s", builder.url)
		impl.pool = newPool(builder.url, builder.dialOptions)
	}
	return impl
}

func (store *redisDataStoreImpl) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	c := store.getConn()
	defer c.Close() // nolint:errcheck

	jsonStr, err := r.String(c.Do("HGET", store.featuresKey(kind), key))

	if err != nil {
		if err == r.ErrNil {
			store.loggers.Debugf("Key: %s not found in \"%s\"", key, kind.GetNamespace())
			return nil, nil
		}
		return nil, err
	}

	item, jsonErr := utils.UnmarshalItem(kind, []byte(jsonStr))
	if jsonErr != nil {
		return nil, fmt.Errorf("failed to unmarshal %s key %s: %s", kind, key, jsonErr)
	}
	return item, nil
}

func (store *redisDataStoreImpl) GetAll(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	results := make(map[string]interfaces.VersionedData)

	c := store.getConn()
	defer c.Close() // nolint:errcheck

	values, err := r.StringMap(c.Do("HGETALL", store.featuresKey(kind)))

	if err != nil && err != r.ErrNil {
		return nil, err
	}

	for k, v := range values {
		item, jsonErr := utils.UnmarshalItem(kind, []byte(v))

		if jsonErr != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %s", kind, err)
		}

		results[k] = item
	}
	return results, nil
}

func (store *redisDataStoreImpl) Init(allData []interfaces.StoreCollection) error {
	c := store.getConn()
	defer c.Close() // nolint:errcheck

	_ = c.Send("MULTI")

	for _, coll := range allData {
		baseKey := store.featuresKey(coll.Kind)

		_ = c.Send("DEL", baseKey)

		for _, item := range coll.Items {
			key := item.GetKey()
			data, jsonErr := json.Marshal(item)

			if jsonErr != nil {
				return fmt.Errorf("failed to marshal %s key %s: %s", coll.Kind, key, jsonErr)
			}

			_ = c.Send("HSET", baseKey, key, data)
		}
	}

	_ = c.Send("SET", store.initedKey(), "")

	_, err := c.Do("EXEC")

	return err
}

func (store *redisDataStoreImpl) Upsert(kind interfaces.VersionedDataKind, newItem interfaces.VersionedData) (interfaces.VersionedData, error) {
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

		oldItem, err := store.Get(kind, key)

		if err != nil {
			return nil, err
		}

		if oldItem != nil && oldItem.GetVersion() >= newItem.GetVersion() {
			updateOrDelete := "update"
			if newItem.IsDeleted() {
				updateOrDelete = "delete"
			}
			store.loggers.Debugf(`Attempted to %s key: %s version: %d in "%s" with a version that is the same or older: %d`,
				updateOrDelete, key, oldItem.GetVersion(), kind.GetNamespace(), newItem.GetVersion())
			return oldItem, nil
		}

		data, jsonErr := json.Marshal(newItem)
		if jsonErr != nil {
			return nil, fmt.Errorf("failed to marshal %s key %s: %s", kind, key, jsonErr)
		}

		_ = c.Send("MULTI")
		err = c.Send("HSET", baseKey, key, data)
		if err == nil {
			var result interface{}
			result, err = c.Do("EXEC")
			if err == nil {
				if result == nil {
					// if exec returned nothing, it means the watch was triggered and we should retry
					store.loggers.Debug("Concurrent modification detected, retrying")
					continue
				}
			}
			return newItem, nil
		}
		return nil, err
	}
}

func (store *redisDataStoreImpl) IsInitialized() bool {
	c := store.getConn()
	defer c.Close() // nolint:errcheck
	inited, _ := r.Bool(c.Do("EXISTS", store.initedKey()))
	return inited
}

func (store *redisDataStoreImpl) IsStoreAvailable() bool {
	c := store.getConn()
	defer c.Close() // nolint:errcheck
	_, err := r.Bool(c.Do("EXISTS", store.initedKey()))
	return err == nil
}

func (store *redisDataStoreImpl) Close() error {
	// The Redis client doesn't currently need to be explicitly disposed of
	return nil
}

func (store *redisDataStoreImpl) featuresKey(kind interfaces.VersionedDataKind) string {
	return store.prefix + ":" + kind.GetNamespace()
}

func (store *redisDataStoreImpl) initedKey() string {
	return store.prefix + ":" + initedKey
}

func (store *redisDataStoreImpl) getConn() r.Conn {
	return store.pool.Get()
}

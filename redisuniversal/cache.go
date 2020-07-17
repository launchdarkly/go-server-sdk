package redisuniversal

import (
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis"
	"gopkg.in/launchdarkly/go-server-sdk.v4"
	"gopkg.in/launchdarkly/go-server-sdk.v4/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v4/utils"
	"os"
	"os/signal"
	"time"
)

type cache struct {
	loggers ldlog.Loggers
	config  Options
	client  redis.UniversalClient
}

func newRedisCache(options Options, loggers ldlog.Loggers) cache {
	loggers.SetPrefix("RedisFeatureStore:")
	client := redis.NewUniversalClient(options.CacheOpts)
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, os.Kill)
	go func() {
		select {
		case <-c:
			_ = client.Close()
		}
	}()
	if options.MaxRetryCount == 0 {
		options.MaxRetryCount = defaultRetryCount
	}
	return cache{
		loggers: loggers,
		client:  client,
		config:  options,
	}
}
func (c cache) featuresKey(kind ldclient.VersionedDataKind) string {
	return c.config.CachePrefix + ":" + kind.GetNamespace()
}

func (c cache) initedKey() string {
	return c.config.CachePrefix + ":" + initedKey
}

func (c cache) GetInternal(kind ldclient.VersionedDataKind, key string) (ldclient.VersionedData, error) {
	jsonStr, err := c.client.HGet(c.featuresKey(kind), key).Result()
	if err != nil {
		if err == redis.Nil {
			c.loggers.Debugf("Key: %s not found in \"%s\"", key, kind.GetNamespace())
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

func (c cache) GetAllInternal(kind ldclient.VersionedDataKind) (map[string]ldclient.VersionedData, error) {
	values, err := c.client.HGetAll(c.featuresKey(kind)).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	results := make(map[string]ldclient.VersionedData)
	for k, v := range values {
		item, jsonErr := utils.UnmarshalItem(kind, []byte(v))
		if jsonErr != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %s", kind, err)
		}

		results[k] = item
	}
	return results, nil
}

func (c cache) UpsertInternal(kind ldclient.VersionedDataKind, newItem ldclient.VersionedData) (ldclient.VersionedData, error) {
	baseKey := c.featuresKey(kind)
	key := newItem.GetKey()
	var item ldclient.VersionedData
	availableRetries := 10
	var retryErr error
	for availableRetries > 0 {
		// Avoid failing in an infinite loop, we try to "upsert" te value a finite number of times,
		//	if it can't be done after the specified retry limit, we return error
		availableRetries--
		err := c.client.Watch(func(tx *redis.Tx) error {
			oldItem, err := c.GetInternal(kind, key)
			if err != nil {
				return err
			}

			if oldItem != nil && oldItem.GetVersion() >= newItem.GetVersion() {
				updateOrDelete := "update"
				if newItem.IsDeleted() {
					updateOrDelete = "delete"
				}
				c.loggers.Debugf(`Attempted to %s key: %s version: %d in "%s" with a version that is the same or older: %d`,
					updateOrDelete, key, oldItem.GetVersion(), kind.GetNamespace(), newItem.GetVersion())
				item = oldItem
				return nil
			}

			data, jsonErr := json.Marshal(newItem)
			if jsonErr != nil {
				return fmt.Errorf("failed to marshal %s key %s: %s", kind, key, jsonErr)
			}

			result, err := tx.TxPipelined(func(pipe redis.Pipeliner) error {
				err = pipe.HSet(baseKey, key, data).Err()
				if err == nil {
					result, err := pipe.Exec()
					// if exec returned nothing, it means the watch was triggered and we should retry
					if err == nil && len(result) > 0 {
						c.loggers.Debug("Concurrent modification detected, retrying")
						return nil
					}
					item = newItem
				}
				return nil // end Pipeline
			})
			if err != nil {
				return err // Pipeline error
			}
			if len(result) > 0 {
				return result[0].Err() // Pipeline failed
			}
			return nil //end WATCH
		}, baseKey)
		if err != nil {
			return nil, err
		}
		if item != nil {
			return item, nil
		}
	}
	return nil, retryErr
}

func (c cache) InitializedInternal() bool {
	inited, _ := c.client.Exists(c.initedKey()).Result()
	return inited == 1
}

func (c cache) GetCacheTTL() time.Duration {
	return c.config.CacheTTL
}

func (c cache) InitInternal(allData map[ldclient.VersionedDataKind]map[string]ldclient.VersionedData) error {
	pipe := c.client.Pipeline()
	for kind, items := range allData {
		baseKey := c.featuresKey(kind)
		if err := pipe.Del(baseKey).Err(); err != nil {
			return err
		}

		for k, v := range items {
			data, jsonErr := json.Marshal(v)
			if jsonErr != nil {
				return fmt.Errorf("failed to marshal %s key %s: %s", kind, k, jsonErr)
			}

			if err := pipe.HSet(baseKey, k, data).Err(); err != nil {
				return err
			}
		}
	}

	if err := pipe.Set(c.initedKey(), "", 0).Err(); err != nil {
		return err
	}
	_, err := pipe.Exec()
	return err
}

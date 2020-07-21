package redisuniversal

import (
"encoding/json"
"strings"
"fmt"
"os"
"os/signal"
"time"

"github.com/go-redis/redis"
"gopkg.in/launchdarkly/go-server-sdk.v4"
"gopkg.in/launchdarkly/go-server-sdk.v4/ldlog"
"gopkg.in/launchdarkly/go-server-sdk.v4/utils"
)

type redisStoreCore struct {
	loggers ldlog.Loggers
	config  Options
	client  redis.UniversalClient
	clusterMode bool
}

const hashTag = "{ld}."

func newRedisStoreCore(options Options, loggers ldlog.Loggers) *redisStoreCore {
	loggers.SetPrefix("RedisFeatureStore:")
	client := redis.NewUniversalClient(options.RedisOpts)

	// ping the server so we know we are good
	err := client.Ping().Err()
	if err != nil {
		loggers.Error("could not ping redis server: %v", err)
		return nil
	}

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
	if options.CacheTTL == 0 {
		options.CacheTTL = DefaultCacheTTL
	}
	var clusterMode bool
	if len(options.RedisOpts.Addrs) > 1 {
		clusterMode = true
	}

	return &redisStoreCore{
		loggers: loggers,
		client:  client,
		config:  options,
		clusterMode: clusterMode,
	}
}
func (c *redisStoreCore) featuresKey(kind ldclient.VersionedDataKind) string {
	return c.hashTagKey(c.config.Prefix + ":" + kind.GetNamespace())
}

func (c *redisStoreCore) initedKey() string {
	return c.config.Prefix + ":" + initedKey
}

// We use a hashtag in order to keep all keys in the same node (and hash slot) so we can perform
// and use watch ... exec without issues. Only in ClusterMode
func (c *redisStoreCore) hashTagKey(key string) string {
	if c.clusterMode {
		return hashTag + key
	}
	return key
}

// Removing the hashtag from the key
func (c *redisStoreCore) cleanHashTagKey(key string) string {
	if c.clusterMode {
		return strings.Replace(key, hashTag, "",-1)
	}
	return key
}

func (c *redisStoreCore) GetInternal(kind ldclient.VersionedDataKind, key string) (ldclient.VersionedData, error) {
	jsonStr, err := c.client.HGet(c.featuresKey(kind), c.hashTagKey(key)).Result()
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

func (c *redisStoreCore) GetAllInternal(kind ldclient.VersionedDataKind) (map[string]ldclient.VersionedData, error) {
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

		results[c.cleanHashTagKey(k)] = item
	}
	return results, nil
}

func (c *redisStoreCore) UpsertInternal(kind ldclient.VersionedDataKind, newItem ldclient.VersionedData) (ldclient.VersionedData, error) {
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
				err = pipe.HSet(baseKey, c.hashTagKey(key), data).Err()
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

func (c *redisStoreCore) InitializedInternal() bool {
	inited, _ := c.client.Exists(c.initedKey()).Result()
	return inited == 1
}

func (c *redisStoreCore) IsStoreAvailable() bool {
	_, err := c.client.Exists(c.initedKey()).Result()
	return err == nil
}

func (c *redisStoreCore) GetCacheTTL() time.Duration {
	return c.config.CacheTTL
}

func (c *redisStoreCore) GetDiagnosticsComponentTypeName() string {
	return "Redis"
}

func (c *redisStoreCore) InitInternal(allData map[ldclient.VersionedDataKind]map[string]ldclient.VersionedData) error {
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

			if err := pipe.HSet(baseKey, c.hashTagKey(k), data).Err(); err != nil {
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

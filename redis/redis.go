package redis

import (
	"encoding/json"
	r "github.com/garyburd/redigo/redis"
	ld "github.com/launchdarkly/go-client"
	"strconv"
	"time"
)

// A Redis-backed feature store.
type RedisFeatureStore struct {
	prefix string
	pool   *r.Pool
}

var pool *r.Pool

func newPool(host string, port int) *r.Pool {
	pool = &r.Pool{
		MaxIdle:     20,
		IdleTimeout: 300 * time.Second,
		Dial: func() (c r.Conn, err error) {
			c, err = r.Dial("tcp", host+":"+strconv.Itoa(port))
			return
		},
		TestOnBorrow: func(c r.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	return pool
}

func (store *RedisFeatureStore) getConn() r.Conn {
	return store.pool.Get()
}

// Constructs a new Redis-backed feature store connecting to the specified host and port.
// Attaches a prefix string to all keys to namespace LaunchDarkly-specific keys. If the
// specified prefix is the empty string, it defaults to "launchdarkly"
func NewRedisFeatureStore(host string, port int, prefix string) *RedisFeatureStore {
	pool := newPool(host, port)

	if prefix == "" {
		prefix = "launchdarkly"
	}

	store := RedisFeatureStore{
		prefix: prefix,
		pool:   pool,
	}

	return &store
}

func (store *RedisFeatureStore) featuresKey() string {
	return store.prefix + ":features"
}

func (store *RedisFeatureStore) Get(key string) (*ld.Feature, error) {
	var feature ld.Feature

	c := store.getConn()
	defer c.Close()

	jsonStr, err := r.String(c.Do("HGET", store.featuresKey(), key))

	if err != nil {
		if err == r.ErrNil {
			return nil, nil
		}
		return nil, err
	}

	if jsonErr := json.Unmarshal([]byte(jsonStr), &feature); jsonErr != nil {
		return nil, jsonErr
	}

	if feature.Deleted {
		return nil, nil
	}

	return &feature, nil
}

func (store *RedisFeatureStore) All() (map[string]*ld.Feature, error) {
	var feature ld.Feature

	results := make(map[string]*ld.Feature)

	c := store.getConn()
	defer c.Close()

	values, err := r.StringMap(c.Do("HGETALL", store.featuresKey()))

	if err != nil && err != r.ErrNil {
		return nil, err
	}

	for k, v := range values {
		jsonErr := json.Unmarshal([]byte(v), &feature)

		if jsonErr != nil {
			return nil, err
		}

		if !feature.Deleted {
			results[k] = &feature
		}
	}
	return results, nil
}

func (store *RedisFeatureStore) Init(features map[string]*ld.Feature) error {
	c := store.getConn()
	defer c.Close()

	c.Send("MULTI")
	c.Send("DEL", store.featuresKey())

	for k, v := range features {
		data, jsonErr := json.Marshal(v)

		if jsonErr != nil {
			return jsonErr
		}

		c.Send("HSET", store.featuresKey(), k, data)
	}
	_, err := c.Do("EXEC")
	return err
}

func (store *RedisFeatureStore) Delete(key string, version int) error {
	c := store.getConn()
	defer c.Close()

	c.Send("WATCH", store.featuresKey())
	defer c.Send("UNWATCH")

	feature, featureErr := store.Get(key)

	if featureErr != nil {
		return featureErr
	}

	if feature != nil && feature.Version >= version {
		return nil
	}

	feature.Deleted = true
	feature.Version = version

	data, jsonErr := json.Marshal(feature)

	if jsonErr != nil {
		return jsonErr
	}

	_, err := c.Do("HSET", store.featuresKey(), data)

	return err
}

func (store *RedisFeatureStore) Upsert(key string, f ld.Feature) error {
	c := store.getConn()
	defer c.Close()

	c.Send("WATCH", store.featuresKey())
	defer c.Send("UNWATCH")

	o, featureErr := store.Get(key)

	if featureErr != nil {
		return featureErr
	}

	if o.Version >= f.Version {
		return nil
	}

	data, jsonErr := json.Marshal(f)

	if jsonErr != nil {
		return jsonErr
	}

	_, err := c.Do("HSET", store.featuresKey(), key, data)

	return err
}

func (store *RedisFeatureStore) Initialized() bool {
	c := store.getConn()
	defer c.Close()

	init, err := r.Bool(c.Do("EXISTS", store.featuresKey()))

	return err == nil && init
}

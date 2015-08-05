package redis

import (
	"encoding/json"
	r "github.com/garyburd/redigo/redis"
	ld "github.com/launchdarkly/go-client"
	"strconv"
	"time"
)

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

		results[k] = &feature
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

func (store *RedisFeatureStore) Delete(key string) error {
	c := store.getConn()
	defer c.Close()

	_, err := c.Do("HDEL", store.featuresKey(), key)

	return err
}

func (store *RedisFeatureStore) Upsert(key string, f ld.Feature) error {
	c := store.getConn()
	defer c.Close()

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

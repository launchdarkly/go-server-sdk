package redis

import (
	"fmt"

	r "github.com/garyburd/redigo/redis"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

const (
	// DefaultURL is the default URL for connecting to Redis, if you use
	// NewRedisDataStoreWithDefaults. You can specify otherwise with the RedisURL option.
	// If you are using the other constructors, you must specify the URL explicitly.
	DefaultURL = "redis://localhost:6379"
	// DefaultPrefix is a string that is prepended (along with a colon) to all Redis keys used
	// by the data store. You can change this value with the Prefix() option for
	// NewRedisDataStoreWithDefaults, or with the "prefix" parameter to the other constructors.
	DefaultPrefix = "launchdarkly"
)

// DataStore returns a configurable builder for a Redis-backed data store.
func DataStore() *RedisDataStoreBuilder {
	return &RedisDataStoreBuilder{
		prefix: DefaultPrefix,
		url:    DefaultURL,
	}
}

// RedisDataStoreBuilder is a builder for configuring the Redis-based persistent data store.
//
// Obtain an instance of this type by calling DataStore(). After calling its methods to specify any
// desired custom settings, wrap it in a PersistentDataStoreBuilder by calling
// ldcomponents.PersistentDataStore(), and then store this in the SDK configuration's DataStore field.
//
// Builder calls can be chained, for example:
//
//     config.DataStore = redis.DataStore().URL("redis://hostname").Prefix("prefix")
//
// You do not need to call the builder's CreatePersistentDataStore() method yourself to build the
// actual data store; that will be done by the SDK.
type RedisDataStoreBuilder struct {
	prefix      string
	pool        *r.Pool
	url         string
	dialOptions []r.DialOption
}

// Prefix specifies a string that should be prepended to all Redis keys used by the data store.
// A colon will be added to this automatically. If this is unspecified or empty, DefaultPrefix will be used.
func (b *RedisDataStoreBuilder) Prefix(prefix string) *RedisDataStoreBuilder {
	if prefix == "" {
		prefix = DefaultPrefix
	}
	b.prefix = prefix
	return b
}

// URL specifies the Redis host URL. If not specified, the default value is DefaultURL.
//
// Note that some Redis client features can also be specified as part of the URL: Redigo supports
// the redis:// syntax (https://www.iana.org/assignments/uri-schemes/prov/redis), which can include a
// password and a database number, as well as rediss://
// (https://www.iana.org/assignments/uri-schemes/prov/rediss), which enables TLS.
func (b *RedisDataStoreBuilder) URL(url string) *RedisDataStoreBuilder {
	if url == "" {
		url = DefaultURL
	}
	b.url = url
	return b
}

// HostAndPort is a shortcut for specifying the Redis host address as a hostname and port.
func (b *RedisDataStoreBuilder) HostAndPort(host string, port int) *RedisDataStoreBuilder {
	return b.URL(fmt.Sprintf("redis://%s:%d", host, port))
}

// Pool specifies that the data store should use a specific connection pool configuration. If not
// specified, it will create a default configuration (see package description). Specifying this
// option will cause any address specified with URL() or HostAndPort() to be ignored.
//
// If you only need to change basic connection options such as providing a password, it is
// simpler to use DialOptions().
func (b *RedisDataStoreBuilder) Pool(pool *r.Pool) *RedisDataStoreBuilder {
	b.pool = pool
	return b
}

// DialOptions specifies any of the advanced Redis connection options supported by Redigo, such as
// DialPassword.
//
//     import (
//         redigo "github.com/garyburd/redigo/redis"
//         "gopkg.in/launchdarkly/go-server-sdk.v5/redis"
//     )
//     factory, err := redis.NewRedisDataStoreFactory(redis.DialOption(redigo.DialPassword("verysecure123")))
//
// Note that some Redis client features can also be specified as part of the URL: see  URL().
func (b *RedisDataStoreBuilder) DialOptions(options ...r.DialOption) *RedisDataStoreBuilder {
	b.dialOptions = options
	return b
}

// CreatePersistentDataStore is called internally by the SDK to create the data store implementation object.
func (b *RedisDataStoreBuilder) CreatePersistentDataStore(context interfaces.ClientContext) (interfaces.PersistentDataStore, error) {
	store := newRedisDataStoreImpl(b, context.GetLoggers())
	return store, nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *RedisDataStoreBuilder) DescribeConfiguration() ldvalue.Value {
	return ldvalue.String("Redis")
}

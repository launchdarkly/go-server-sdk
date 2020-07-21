package redisuniversal

import (
"github.com/go-redis/redis"
"gopkg.in/launchdarkly/go-server-sdk.v4"
"gopkg.in/launchdarkly/go-server-sdk.v4/ldlog"
"gopkg.in/launchdarkly/go-server-sdk.v4/utils"
"time"
)

const (
	initedKey         = "$inited"
	defaultRetryCount = 10
	// DefaultCacheTTL is the default amount of time that recently read or updated items will
	// be cached in memory, if you use NewRedisFeatureStoreWithDefaults. You can specify otherwise
	// with the CacheTTL option. If you are using the other constructors, their "timeout"
	// parameter serves the same purpose and there is no default.
	DefaultCacheTTL = 15 * time.Second
)

// Options will hold all the configuration values used in the RedisStoreCore
type Options struct {
	RedisOpts     *redis.UniversalOptions
	Prefix   	string
	CacheTTL      time.Duration
	MaxRetryCount int
}

type featureStore struct {
	options Options
	loggers ldlog.Loggers
	wrapper *utils.FeatureStoreWrapper
}

// NewRedisFeatureStoreFactory returns a factory function for a Redis-backed feature store.
func NewRedisFeatureStoreFactory(config Options) (ldclient.FeatureStoreFactory, error) {
	return func(ldConfig ldclient.Config) (ldclient.FeatureStore, error) {
		return featureStore{
			loggers: ldConfig.Loggers,
			wrapper: utils.NewFeatureStoreWrapperWithConfig(newRedisStoreCore(config, ldConfig.Loggers), ldConfig),
		}, nil
	}, nil
}

func (s featureStore) Get(kind ldclient.VersionedDataKind, key string) (ldclient.VersionedData, error) {
	return s.wrapper.Get(kind, key)
}

func (s featureStore) All(kind ldclient.VersionedDataKind) (map[string]ldclient.VersionedData, error) {
	return s.wrapper.All(kind)
}

func (s featureStore) Init(allData map[ldclient.VersionedDataKind]map[string]ldclient.VersionedData) error {
	return s.wrapper.Init(allData)
}

func (s featureStore) Delete(kind ldclient.VersionedDataKind, key string, version int) error {
	return s.wrapper.Delete(kind, key, version)
}

func (s featureStore) Upsert(kind ldclient.VersionedDataKind, item ldclient.VersionedData) error {
	return s.wrapper.Upsert(kind, item)
}

func (s featureStore) Initialized() bool {
	return s.wrapper.Initialized()
}


package ldcomponents

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// PersistentDataStoreDefaultCacheTime is the default amount of time that recently read or updated items
// will be cached in memory, if you use PersistentDataStore(). You can specify otherwise with the
// PersistentDataStoreBuilder.CacheTime() option.
const PersistentDataStoreDefaultCacheTime = 15 * time.Second

// PersistentDataStore returns a configuration builder for some implementation of a persistent data store.
//
// The return value of this function should be stored in the DataStore field of ld.Config.
//
// This method is used in conjunction with another configuration builder object provided by specific
// packages such as the Redis integration. Each LaunchDarkly Go SDK database integration has a
// DataStore() method that returns a configuration builder, with builder methods for options that
// are specific to that database. The SDK also provides some universal behaviors for all persistent
// data stores, such as caching; PersistantDataStoreBuilder provides methods to configure those
// behaviors. For instance, in this example, URL() is an option that is specific to the Redis
// integration, whereas CacheSeconds is not specific to Redis:
//
//	config := ld.Config{
//	    DataStore: ldcomponents.PersistentDataStore(
//	        ldredis.DataStore().URL("redis://my-redis-host"),
//	    ).CacheSeconds(15),
//	}
//
// See PersistentDataStoreBuilder for more on how this method is used.
//
// For more information on the available persistent data store implementations, see the reference
// guide on "Persistent data stores": https://docs.launchdarkly.com/sdk/concepts/data-stores
func PersistentDataStore(
	persistentDataStoreFactory subsystems.ComponentConfigurer[subsystems.PersistentDataStore],
) *PersistentDataStoreBuilder {
	return &PersistentDataStoreBuilder{
		persistentDataStoreFactory: persistentDataStoreFactory,
		cacheTTL:                   PersistentDataStoreDefaultCacheTime,
	}
}

// PersistentDataStoreBuilder is a configurable factory for a persistent data store.
//
// Each LaunchDarkly Go SDK database integration has its own configuration builder, with builder methods
// for options that are specific to that database. The SDK also provides some universal behaviors for all
// persistent data stores, such as caching; PersistantDataStoreBuilder provides methods to configure those
// behaviors. For instance, in this example, URL() is an option that is specific to the Redis
// integration, whereas CacheSeconds is not specific to Redis:
//
//	config := ld.Config{
//	    DataStore: ldcomponents.PersistentDataStore(
//	        ldredis.DataStore().URL("redis://my-redis-host"),
//	    ).CacheSeconds(15),
//	}
type PersistentDataStoreBuilder struct {
	persistentDataStoreFactory subsystems.ComponentConfigurer[subsystems.PersistentDataStore]
	cacheTTL                   time.Duration
}

// CacheTime specifies the cache TTL. Items will be evicted from the cache after this amount of time
// from the time when they were originally cached.
//
// If the value is zero, caching is disabled (equivalent to NoCaching).
//
// If the value is negative, data is cached forever (equivalent to CacheForever).
func (b *PersistentDataStoreBuilder) CacheTime(cacheTime time.Duration) *PersistentDataStoreBuilder {
	b.cacheTTL = cacheTime
	return b
}

// CacheSeconds is a shortcut for calling CacheTime with a duration in seconds.
func (b *PersistentDataStoreBuilder) CacheSeconds(cacheSeconds int) *PersistentDataStoreBuilder {
	return b.CacheTime(time.Duration(cacheSeconds) * time.Second)
}

// CacheForever specifies that the in-memory cache should never expire. In this mode, data will be
// written to both the underlying persistent store and the cache, but will only ever be read from the
// persistent store if the SDK is restarted.
//
// Use this mode with caution: it means that in a scenario where multiple processes are sharing
// the database, and the current process loses connectivity to LaunchDarkly while other processes
// are still receiving updates and writing them to the database, the current process will have
// stale data.
func (b *PersistentDataStoreBuilder) CacheForever() *PersistentDataStoreBuilder {
	return b.CacheTime(-1 * time.Millisecond)
}

// NoCaching specifies that the SDK should not use an in-memory cache for the persistent data store.
// This means that every feature flag evaluation will trigger a data store query.
func (b *PersistentDataStoreBuilder) NoCaching() *PersistentDataStoreBuilder {
	return b.CacheTime(0)
}

// Build is called internally by the SDK.
func (b *PersistentDataStoreBuilder) Build(clientContext subsystems.ClientContext) (subsystems.DataStore, error) {
	core, err := b.persistentDataStoreFactory.Build(clientContext)
	if err != nil {
		return nil, err
	}
	return datastore.NewPersistentDataStoreWrapper(core, clientContext.GetDataStoreUpdateSink(), b.cacheTTL,
		clientContext.GetLogging().Loggers), nil
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *PersistentDataStoreBuilder) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	if dd, ok := b.persistentDataStoreFactory.(subsystems.DiagnosticDescription); ok {
		return dd.DescribeConfiguration(context)
	}
	return ldvalue.String("custom")
}

package ldcomponents

import (
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v6/ldcomponents/ldstoreimpl"
)

// DefaultBigSegmentsUserCacheSize is the default value for
// BigSegmentsConfigurationBuilder.UserCacheSize.
const DefaultBigSegmentsUserCacheSize = 1000

// DefaultBigSegmentsUserCacheTime is the default value for
// BigSegmentsConfigurationBuilder.UserCacheTime.
const DefaultBigSegmentsUserCacheTime = time.Second * 5

// DefaultBigSegmentsStatusPollInterval is the default value for
// BigSegmentsConfigurationBuilder.StatusPollInterval.
const DefaultBigSegmentsStatusPollInterval = time.Second * 5

// DefaultBigSegmentsStaleAfter is the default value for
// BigSegmentsConfigurationBuilder.StaleAfter.
const DefaultBigSegmentsStaleAfter = time.Second * 120

// BigSegmentsConfigurationBuilder contains methods for configuring the SDK's Big Segments behavior.
//
// "Big Segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
//
// If you want to set non-default values for any of these properties, create a builder with
// ldcomponents.BigSegments(), change its properties with the BigSegmentsConfigurationBuilder
// methods, and store it in Config.BigSegments:
//
//     config := ld.Config{
//         BigSegments: ldcomponents.BigSegments(ldredis.DataStore()).
//             UserCacheSize(2000).
//		       StaleAfter(time.Second * 60),
//     }
//
// You only need to use the methods of BigSegmentsConfigurationBuilder if you want to customize
// options other than the data store itself.
type BigSegmentsConfigurationBuilder struct {
	storeFactory interfaces.BigSegmentStoreFactory
	config       ldstoreimpl.BigSegmentsConfigurationProperties
}

// BigSegments returns a configuration builder for the SDK's Big Segments feature.
//
// "Big Segments" are a specific type of user segments. For more information, read the LaunchDarkly
// documentation about user segments: https://docs.launchdarkly.com/home/users
//
// After configuring this object, store it in the BigSegments field of your SDK configuration. For
// example, using the Redis integration:
//
//     config := ld.Config{
//         BigSegments: ldcomponents.BigSegments(ldredis.DataStore().Prefix("app1")).
//             UserCacheSize(2000),
//     }
//
// You must always specify the storeFactory parameter, to tell the SDK what database you are using.
// Several database integrations exist for the LaunchDarkly SDK, each with its own behavior and options
// specific to that database; this is described via some implementation of BigSegmentStoreFactory.
// The BigSegmentsConfigurationBuilder adds configuration options for aspects of SDK behavior
// that are independent of the database. In the example above, Prefix() is an option specifically for the
// Redis integration, whereas UserCacheSize() is an option that can be used for any data store type.
//
// If you do not set Config.BigSegments-- or if you pass a nil storeFactory to this function-- the
// Big Segments feature will be disabled, and any feature flags that reference a Big Segment will
// behave as if the user was not included in the segment.
func BigSegments(storeFactory interfaces.BigSegmentStoreFactory) *BigSegmentsConfigurationBuilder {
	return &BigSegmentsConfigurationBuilder{
		storeFactory: storeFactory,
		config: ldstoreimpl.BigSegmentsConfigurationProperties{
			UserCacheSize:      DefaultBigSegmentsUserCacheSize,
			UserCacheTime:      DefaultBigSegmentsUserCacheTime,
			StatusPollInterval: DefaultBigSegmentsStatusPollInterval,
			StaleAfter:         DefaultBigSegmentsStaleAfter,
		},
	}
}

// UserCacheSize sets the maximum number of users whose Big Segment state will be cached by the SDK
// at any given time. The default value is DefaultBigSegmentsUserCacheSize.
//
// To reduce database traffic, the SDK maintains a least-recently-used cache by user key. When a feature
// flag that references a Big Segment is evaluated for some user who is not currently in the cache, the
// SDK queries the database for all Big Segment memberships of that user, and stores them together in a
// single cache entry. If the cache is full, the oldest entry is dropped.
//
// A higher value for UserCacheSize means that database queries for Big Segments will be done less often
// for recently-referenced users, if the application has many users, at the cost of increased memory
// used by the cache.
//
// Cache entries can also expire based on the setting of UserCacheTime.
func (b *BigSegmentsConfigurationBuilder) UserCacheSize(
	userCacheSize int,
) *BigSegmentsConfigurationBuilder {
	b.config.UserCacheSize = userCacheSize
	return b
}

// UserCacheTime sets the maximum length of time that the Big Segment state for a user will be cached
// by the SDK. The default value is DefaultBigSegmentsUserCacheTime.
//
// See UserCacheSize for more about this cache. A higher value for UserCacheTime means that database queries
// for the Big Segment state of any given user will be done less often, but that changes to segment
// membership may not be detected as soon.
func (b *BigSegmentsConfigurationBuilder) UserCacheTime(
	userCacheTime time.Duration,
) *BigSegmentsConfigurationBuilder {
	b.config.UserCacheTime = userCacheTime
	return b
}

// StatusPollInterval sets the interval at which the SDK will poll the Big Segment store to make sure
// it is available and to determine how long ago it was updated. The default value is
// DefaultBigSegmentsStatusPollInterval.
func (b *BigSegmentsConfigurationBuilder) StatusPollInterval(
	statusPollInterval time.Duration,
) *BigSegmentsConfigurationBuilder {
	if statusPollInterval <= 0 {
		statusPollInterval = DefaultBigSegmentsStatusPollInterval
	}
	b.config.StatusPollInterval = statusPollInterval
	return b
}

// StaleAfter sets the maximum length of time between updates of the Big Segments data before the data
// is considered out of date. The default value is DefaultBigSegmentsStaleAfter.
//
// Normally, the LaunchDarkly Relay Proxy updates a timestamp in the Big Segments store at intervals to
// confirm that it is still in sync with the LaunchDarkly data, even if there have been no changes to the
// data. If the timestamp falls behind the current time by the amount specified in StaleAfter, the SDK
// assumes that something is not working correctly in this process and that the data may not be accurate.
//
// While in a stale state, the SDK will still continue using the last known data,
// but LDClient.GetBigSegmentsStoreStatusProvider().GetStatus() will return true in its Stale property,
// and any ldreason.EvaluationReason generated from a feature flag that references a Big Segment will
// have an BigSegmentsStatus of ldreason.BigSegmentsStale.
func (b *BigSegmentsConfigurationBuilder) StaleAfter(
	staleAfter time.Duration,
) *BigSegmentsConfigurationBuilder {
	b.config.StaleAfter = staleAfter
	return b
}

// CreateBigSegmentsConfiguration is called internally by the SDK.
func (b *BigSegmentsConfigurationBuilder) CreateBigSegmentsConfiguration(
	context interfaces.ClientContext,
) (interfaces.BigSegmentsConfiguration, error) {
	config := b.config
	if b.storeFactory != nil {
		store, err := b.storeFactory.CreateBigSegmentStore(context)
		if err != nil {
			return nil, err
		}
		config.Store = store
	}
	return config, nil
}

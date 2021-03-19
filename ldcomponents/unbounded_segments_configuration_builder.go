package ldcomponents

import (
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// DefaultUnboundedSegmentsUserCacheSize is the default value for
// UnboundedSegmentsConfigurationBuilder.UserCacheSize.
const DefaultUnboundedSegmentsUserCacheSize = 1000

// DefaultUnboundedSegmentsUserCacheTime is the default value for
// UnboundedSegmentsConfigurationBuilder.UserCacheTime.
const DefaultUnboundedSegmentsUserCacheTime = time.Second * 5

// DefaultUnboundedSegmentsStatusPollInterval is the default value for
// UnboundedSegmentsConfigurationBuilder.StatusPollInterval.
const DefaultUnboundedSegmentsStatusPollInterval = time.Second * 5

// DefaultUnboundedSegmentsStaleAfter is the default value for
// UnboundedSegmentsConfigurationBuilder.StaleAfter.
const DefaultUnboundedSegmentsStaleAfter = time.Second * 120

type unboundedSegmentsConfigurationImpl struct {
	store              interfaces.UnboundedSegmentStore
	userCacheSize      int
	userCacheTime      time.Duration
	statusPollInterval time.Duration
	staleAfter         time.Duration
}

func (c unboundedSegmentsConfigurationImpl) GetStore() interfaces.UnboundedSegmentStore {
	return c.store
}

func (c unboundedSegmentsConfigurationImpl) GetUserCacheSize() int { return c.userCacheSize }

func (c unboundedSegmentsConfigurationImpl) GetUserCacheTime() time.Duration { return c.userCacheTime }

func (c unboundedSegmentsConfigurationImpl) GetStatusPollInterval() time.Duration {
	return c.statusPollInterval
}

func (c unboundedSegmentsConfigurationImpl) GetStaleAfter() time.Duration { return c.staleAfter }

// UnboundedSegmentsConfigurationBuilder contains methods for configuring the SDK's unbounded segments behavior.
//
// If you want to set non-default values for any of these properties, create a builder with
// ldcomponents.UnboundedSegments(), change its properties with the UnboundedSegmentsConfigurationBuilder
// methods, and store it in Config.UnboundedSegments:
//
//     config := ld.Config{
//         UnboundedSegments: ldcomponents.UnboundedSegments(ldredis.DataStore()).
//             UserCacheSize(2000).
//		       StaleAfter(time.Second * 60),
//     }
//
// You only need to use the methods of UnboundedSegmentsConfigurationBuilder if you want to customize
// options other than the data store itself.
type UnboundedSegmentsConfigurationBuilder struct {
	storeFactory interfaces.UnboundedSegmentStoreFactory
	config       unboundedSegmentsConfigurationImpl
}

// UnboundedSegments returns a configuration builder for the SDK's unbounded segments feature.
//
// After configuring this object, store it in the DataSource field of your SDK configuration. For example,
// using the Redis integration:
//
//     config := ld.Config{
//         UnboundedSegments: ldcomponents.UnboundedSegments(ldredis.DataStore().Prefix("app1")).
//             UserCacheSize(2000),
//     }
//
// You must always specify the storeFactory parameter, to tell the SDK what database you are using.
// Several database integrations exist for the LaunchDarkly SDK, each with its own behavior and options
// specific to that database; this is described via some implementation of UnboundedSegmentStoreFactory.
// The UnboundedSegmentsConfigurationBuilder adds configuration options for aspects of SDK behavior
// that are independent of the database. In the example above, Prefix() is an option specifically for the
// Redis integration, whereas UserCacheSize() is an option that can be used for any data store type.
//
// If you do not set Config.UnboundedSegments-- or if you pass a nil storeFactory to this function-- the
// unbounded segments feature will be disabled, and any feature flags that reference an unbounded
// segment will behave as if the user was not included in the segment.
func UnboundedSegments(storeFactory interfaces.UnboundedSegmentStoreFactory) *UnboundedSegmentsConfigurationBuilder {
	return &UnboundedSegmentsConfigurationBuilder{
		storeFactory: storeFactory,
		config: unboundedSegmentsConfigurationImpl{
			userCacheSize:      DefaultUnboundedSegmentsUserCacheSize,
			userCacheTime:      DefaultUnboundedSegmentsUserCacheTime,
			statusPollInterval: DefaultUnboundedSegmentsStatusPollInterval,
			staleAfter:         DefaultUnboundedSegmentsStaleAfter,
		},
	}
}

// UserCacheSize sets the maximum number of users whose unbounded segment state will be cached by the SDK
// at any given time. The default value is DefaultUnboundedSegmentsUserCacheSize.
//
// To reduce database traffic, the SDK maintains a least-recently-used cache by user key. When a feature flag
// that references an unbounded segment is evaluated for some user who is not currently in the cache, the SDK
// queries the database for all unbounded segment memberships of that user, and stores them together in a
// single cache entry. If the cache is full, the oldest entry is dropped.
//
// A higher value for UserCacheSize means that database queries for unbounded segments will be done less
// often for recently-referenced users, if the application has many users, at the cost of increased memory
// used by the cache.
//
// Cache entries can also expire based on the setting of UserCacheTime.
func (b *UnboundedSegmentsConfigurationBuilder) UserCacheSize(
	userCacheSize int,
) *UnboundedSegmentsConfigurationBuilder {
	b.config.userCacheSize = userCacheSize
	return b
}

// UserCacheTime sets the maximum length of time that the unbounded segment state for a user will be cached
// by the SDK. The default value is DefaultUnboundedSegmentsUserCacheTime.
//
// See UserCacheSize for more about this cache. A higher value for UserCacheTime means that database queries
// for the unbounded segment state of any given user will be done less often, but that changes to segment
// membership may not be detected as soon.
func (b *UnboundedSegmentsConfigurationBuilder) UserCacheTime(
	userCacheTime time.Duration,
) *UnboundedSegmentsConfigurationBuilder {
	b.config.userCacheTime = userCacheTime
	return b
}

// StatusPollInterval sets the interval at which the SDK will poll the unbounded segment store to make sure
// it is available and to determine how long ago it was updated. The default value is
// DefaultUnboundedSegmentsStatusPollInterval.
func (b *UnboundedSegmentsConfigurationBuilder) StatusPollInterval(
	statusPollInterval time.Duration,
) *UnboundedSegmentsConfigurationBuilder {
	if statusPollInterval <= 0 {
		statusPollInterval = DefaultUnboundedSegmentsStatusPollInterval
	}
	b.config.statusPollInterval = statusPollInterval
	return b
}

// StaleAfter sets the maximum length of time between updates of the unbounded segments data before the data
// is considered out of date. The default value is DefaultUnboundedSegmentsStaleAfter.
//
// Normally, the LaunchDarkly Relay Proxy updates a timestamp in the unbounded segments store at intervals
// to confirm that it is still in sync with the LaunchDarkly data, even if there have been no changes to the
// data. If the timestamp falls behind the current time by the amount specified in StaleAfter, the SDK assumes
// that something is not working correctly in this process and that the data may not be accurate.
//
// While in a stale state, the SDK will still continue using the last known data,
// but LDClient.GetUnboundedSegmentsStoreStatusProvider().GetStatus() will return true in its Stale property,
// and any ldreason.EvaluationReason generated from a feature flag that references an unbounded segment will
// have an UnboundedSegmentsStatus of ldreason.UnboundedSegmentsStale.
func (b *UnboundedSegmentsConfigurationBuilder) StaleAfter(
	staleAfter time.Duration,
) *UnboundedSegmentsConfigurationBuilder {
	b.config.staleAfter = staleAfter
	return b
}

// CreateUnboundedSegmentsConfiguration is called internally by the SDK.
func (b *UnboundedSegmentsConfigurationBuilder) CreateUnboundedSegmentsConfiguration(
	context interfaces.ClientContext,
) (interfaces.UnboundedSegmentsConfiguration, error) {
	config := b.config
	if b.storeFactory != nil {
		store, err := b.storeFactory.CreateUnboundedSegmentStore(context)
		if err != nil {
			return nil, err
		}
		config.store = store
	}
	return config, nil
}

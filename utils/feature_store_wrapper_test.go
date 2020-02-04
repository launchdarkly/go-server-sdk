package utils

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	ld "gopkg.in/launchdarkly/go-server-sdk.v5"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/shared_test"
)

type testCacheMode string

const (
	testUncached           testCacheMode = "uncached"
	testCached             testCacheMode = "cached"
	testCachedIndefinitely testCacheMode = "cached indefinitely"
)

var configWithoutLogging = ld.Config{Loggers: ldlog.NewDisabledLoggers()}

func (m testCacheMode) isCached() bool {
	return m != testUncached
}

func (m testCacheMode) ttl() time.Duration {
	switch m {
	case testCached:
		return 30 * time.Second
	case testCachedIndefinitely:
		return -1
	default:
		return 0
	}
}

// Test implementation of DataStoreCore
type mockCore struct {
	cacheTTL         time.Duration
	data             map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData
	fakeError        error
	inited           bool
	initQueriedCount int
}

// Test implementation of NonAtomicDataStoreCore - we test this in somewhat less detail
type mockNonAtomicCore struct {
	data []interfaces.StoreCollection
}

// Test implementation of DataStoreCore for request-coalescing tests
type mockCoreWithInstrumentedQueries struct {
	cacheTTL       time.Duration
	data           map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData
	inited         bool
	queryCount     int
	queryDelay     time.Duration
	queryStartedCh chan struct{}
}

func newCore(ttl time.Duration) *mockCore {
	return &mockCore{
		cacheTTL: ttl,
		data:     map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{ld.Features: {}, ld.Segments: {}},
	}
}

func newCoreWithInstrumentedQueries(ttl time.Duration) *mockCoreWithInstrumentedQueries {
	return &mockCoreWithInstrumentedQueries{
		cacheTTL:       ttl,
		data:           map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{ld.Features: {}, ld.Segments: {}},
		queryDelay:     200 * time.Millisecond,
		queryStartedCh: make(chan struct{}, 2),
	}
}

func (c *mockCore) forceSet(kind interfaces.VersionedDataKind, item interfaces.VersionedData) {
	c.data[kind][item.GetKey()] = item
}

func (c *mockCore) forceRemove(kind interfaces.VersionedDataKind, key string) {
	delete(c.data[kind], key)
}

func (c *mockCore) GetCacheTTL() time.Duration {
	return c.cacheTTL
}

func (c *mockCore) InitInternal(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	if c.fakeError != nil {
		return c.fakeError
	}
	c.data = allData
	c.inited = true
	return nil
}

func (c *mockCore) GetInternal(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	return c.data[kind][key], nil
}

func (c *mockCore) GetAllInternal(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	return c.data[kind], nil
}

func (c *mockCore) UpsertInternal(kind interfaces.VersionedDataKind, item interfaces.VersionedData) (interfaces.VersionedData, error) {
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	oldItem := c.data[kind][item.GetKey()]
	if oldItem != nil && oldItem.GetVersion() >= item.GetVersion() {
		return oldItem, nil
	}
	c.data[kind][item.GetKey()] = item
	return item, nil
}

func (c *mockCore) InitializedInternal() bool {
	c.initQueriedCount++
	return c.inited
}

func (c *mockNonAtomicCore) GetCacheTTL() time.Duration {
	return 0
}

func (c *mockNonAtomicCore) InitCollectionsInternal(allData []interfaces.StoreCollection) error {
	c.data = allData
	return nil
}

func (c *mockNonAtomicCore) GetInternal(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	return nil, nil // not used in tests
}

func (c *mockNonAtomicCore) GetAllInternal(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	return nil, nil // not used in tests
}

func (c *mockNonAtomicCore) UpsertInternal(kind interfaces.VersionedDataKind, item interfaces.VersionedData) (interfaces.VersionedData, error) {
	return nil, nil // not used in tests
}

func (c *mockNonAtomicCore) InitializedInternal() bool {
	return false // not used in tests
}

func (c *mockCoreWithInstrumentedQueries) forceSet(kind interfaces.VersionedDataKind, item interfaces.VersionedData) {
	c.data[kind][item.GetKey()] = item
}

func (c *mockCoreWithInstrumentedQueries) GetCacheTTL() time.Duration {
	return c.cacheTTL
}

func (c *mockCoreWithInstrumentedQueries) InitInternal(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	c.data = allData
	c.inited = true
	return nil
}

func (c *mockCoreWithInstrumentedQueries) GetInternal(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	c.queryStartedCh <- struct{}{}
	<-time.After(c.queryDelay)
	return c.data[kind][key], nil
}

func (c *mockCoreWithInstrumentedQueries) GetAllInternal(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	c.queryStartedCh <- struct{}{}
	<-time.After(c.queryDelay)
	return c.data[kind], nil
}

func (c *mockCoreWithInstrumentedQueries) UpsertInternal(kind interfaces.VersionedDataKind, item interfaces.VersionedData) (interfaces.VersionedData, error) {
	oldItem := c.data[kind][item.GetKey()]
	if oldItem != nil && oldItem.GetVersion() >= item.GetVersion() {
		return oldItem, nil
	}
	c.data[kind][item.GetKey()] = item
	return item, nil
}

func (c *mockCoreWithInstrumentedQueries) InitializedInternal() bool {
	return c.inited
}

func TestDataStoreWrapper(t *testing.T) {
	cacheTime := 30 * time.Second

	runTests := func(t *testing.T, name string, test func(t *testing.T, mode testCacheMode, core *mockCore),
		forModes ...testCacheMode) {
		t.Run(name, func(t *testing.T) {
			if len(forModes) == 0 {
				require.True(t, false, "didn't specify any testCacheModes")
			}
			for _, mode := range forModes {
				t.Run(string(mode), func(t *testing.T) {
					test(t, mode, newCore(mode.ttl()))
				})
			}
		})
	}

	runTests(t, "Get", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()
		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ldeval.FeatureFlag{Key: "flag", Version: 2}

		core.forceSet(ld.Features, &flagv1)
		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item)

		core.forceSet(ld.Features, &flagv2)
		item, err = w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Equal(t, &flagv1, item) // returns cached value, does not call getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Get with deleted item", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()
		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1, Deleted: true}
		flagv2 := ldeval.FeatureFlag{Key: "flag", Version: 2, Deleted: false}

		core.forceSet(ld.Features, &flagv1)
		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Nil(t, item) // item is filtered out because Deleted is true

		core.forceSet(ld.Features, &flagv2)
		item, err = w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Nil(t, item) // it used the cached deleted item rather than calling the getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Get with missing item", func(t *testing.T, mode testCacheMode, core *mockCore) {
		mockLog := shared_test.NewMockLoggers()
		w := NewDataStoreWrapperWithConfig(core, ld.Config{Loggers: mockLog.Loggers})
		defer w.Close()
		flag := ldeval.FeatureFlag{Key: "flag", Version: 1}

		item, err := w.Get(ld.Features, flag.Key)
		require.NoError(t, err)
		require.Nil(t, item)

		assert.Nil(t, mockLog.Output[ldlog.Error]) // missing item should *not* be logged as an error by this component

		core.forceSet(ld.Features, &flag)
		item, err = w.Get(ld.Features, flag.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Nil(t, item) // the cache retains a nil result
		} else {
			require.Equal(t, &flag, item) // no caching, calls getter
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "cached Get uses values from Init", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ldeval.FeatureFlag{Key: "flag", Version: 1}

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flagv1.Key: &flagv1},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, core.data, allData)

		core.forceSet(ld.Features, &flagv2)
		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item) // it used the cached item rather than calling the getter
	}, testCached, testCachedIndefinitely)

	runTests(t, "All", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()
		flag1 := ldeval.FeatureFlag{Key: "flag1", Version: 1}
		flag2 := ldeval.FeatureFlag{Key: "flag2", Version: 1}

		core.forceSet(ld.Features, &flag1)
		core.forceSet(ld.Features, &flag2)
		items, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, 2, len(items))

		core.forceRemove(ld.Features, flag2.Key)
		items, err = w.All(ld.Features)
		require.NoError(t, err)
		if mode.isCached() {
			require.Equal(t, 2, len(items))
		} else {
			require.Equal(t, 1, len(items))
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "cached All uses values from Init", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flag1 := ldeval.FeatureFlag{Key: "flag1", Version: 1}
		flag2 := ldeval.FeatureFlag{Key: "flag2", Version: 1}

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flag1.Key: &flag1, flag2.Key: &flag2},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, allData, core.data)

		core.forceRemove(ld.Features, flag2.Key)
		items, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, 2, len(items))
	}, testCached, testCachedIndefinitely)

	runTests(t, "cached All uses fresh values if there has been an update", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flag1 := ldeval.FeatureFlag{Key: "flag1", Version: 1}
		flag1v2 := ldeval.FeatureFlag{Key: "flag1", Version: 2}
		flag2 := ldeval.FeatureFlag{Key: "flag2", Version: 1}
		flag2v2 := ldeval.FeatureFlag{Key: "flag2", Version: 2}

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flag1.Key: &flag1, flag2.Key: &flag2},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, allData, core.data)

		// make a change to flag1 using the wrapper - this should flush the cache
		err = w.Upsert(ld.Features, &flag1v2)
		require.NoError(t, err)

		// make a change to flag2 that bypasses the cache
		core.forceSet(ld.Features, &flag2v2)

		// we should now see both changes since the cache was flushed
		items, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, 2, items[flag2.Key].GetVersion())
	}, testCached)

	runTests(t, "Upsert - successful", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ldeval.FeatureFlag{Key: "flag", Version: 2}

		err := w.Upsert(ld.Features, &flagv1)
		require.NoError(t, err)
		require.Equal(t, &flagv1, core.data[ld.Features][flagv1.Key])

		err = w.Upsert(ld.Features, &flagv2)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[ld.Features][flagv1.Key])

		// if we have a cache, verify that the new item is now cached by writing a different value
		// to the underlying data - Get should still return the cached item
		if mode.isCached() {
			flagv3 := ldeval.FeatureFlag{Key: "flag", Version: 3}
			core.forceSet(ld.Features, &flagv3)
		}

		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv2, item)
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "cached Upsert - unsuccessful", func(t *testing.T, mode testCacheMode, core *mockCore) {
		// This is for an upsert where the data in the store has a higher version. In an uncached
		// store, this is just a no-op as far as the wrapper is concerned so there's nothing to
		// test here. In a cached store, we need to verify that the cache has been refreshed
		// using the data that was found in the store.
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ldeval.FeatureFlag{Key: "flag", Version: 2}

		err := w.Upsert(ld.Features, &flagv2)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[ld.Features][flagv1.Key])

		err = w.Upsert(ld.Features, &flagv1)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[ld.Features][flagv1.Key]) // value in store remains the same

		flagv3 := ldeval.FeatureFlag{Key: "flag", Version: 3}
		core.forceSet(ld.Features, &flagv3) // bypasses cache so we can verify that flagv2 is in the cache

		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv2, item)
	}, testCached, testCachedIndefinitely)

	runTests(t, "Delete", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag1", Version: 1}
		flagv2 := ldeval.FeatureFlag{Key: "flag1", Version: 2, Deleted: true}
		flagv3 := ldeval.FeatureFlag{Key: "flag1", Version: 3}

		core.forceSet(ld.Features, &flagv1)
		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item)

		err = w.Delete(ld.Features, flagv1.Key, 2)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[ld.Features][flagv1.Key])

		// make a change to the flag that bypasses the cache
		core.forceSet(ld.Features, &flagv3)

		item, err = w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Nil(t, item)
		} else {
			require.Equal(t, &flagv3, item)
		}
	}, testUncached, testCached, testCachedIndefinitely)

	t.Run("Initialized calls InitializedInternal only if not already inited", func(t *testing.T) {
		core := newCore(0)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		assert.False(t, w.Initialized())
		assert.Equal(t, 1, core.initQueriedCount)

		core.inited = true
		assert.True(t, w.Initialized())
		assert.Equal(t, 2, core.initQueriedCount)

		core.inited = false
		assert.True(t, w.Initialized())
		assert.Equal(t, 2, core.initQueriedCount)
	})

	t.Run("Initialized won't call InitializedInternal if Init has been called", func(t *testing.T) {
		core := newCore(0)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		assert.False(t, w.Initialized())
		assert.Equal(t, 1, core.initQueriedCount)

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{ld.Features: {}}
		err := w.Init(allData)
		require.NoError(t, err)

		assert.True(t, w.Initialized())
		assert.Equal(t, 1, core.initQueriedCount)
	})

	t.Run("Initialized can cache false result", func(t *testing.T) {
		core := newCore(500 * time.Millisecond)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		assert.False(t, w.Initialized())
		assert.Equal(t, 1, core.initQueriedCount)

		core.inited = true
		assert.False(t, w.Initialized())
		assert.Equal(t, 1, core.initQueriedCount)

		time.Sleep(600 * time.Millisecond)
		assert.True(t, w.Initialized())
		assert.Equal(t, 2, core.initQueriedCount)
	})

	t.Run("Cached Get coalesces requests for same key", func(t *testing.T) {
		core := newCoreWithInstrumentedQueries(cacheTime)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flag := ldeval.FeatureFlag{Key: "flag", Version: 9}
		core.forceSet(ld.Features, &flag)

		resultCh := make(chan int, 2)
		go func() {
			result, _ := w.Get(ld.Features, flag.Key)
			resultCh <- result.GetVersion()
		}()
		// We can't actually *guarantee* that our second query will start while the first one is still
		// in progress, but the combination of waiting on queryStartedCh and the built-in delay in
		// mockCoreWithInstrumentedQueries should make it extremely likely.
		<-core.queryStartedCh
		go func() {
			result, _ := w.Get(ld.Features, flag.Key)
			resultCh <- result.GetVersion()
		}()

		result1 := <-resultCh
		result2 := <-resultCh
		assert.Equal(t, flag.Version, result1)
		assert.Equal(t, flag.Version, result2)

		assert.Equal(t, 0, len(core.queryStartedCh)) // core only received 1 query
	})

	t.Run("Cached Get doesn't coalesce requests for same key", func(t *testing.T) {
		core := newCoreWithInstrumentedQueries(cacheTime)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flag1 := ldeval.FeatureFlag{Key: "flag1", Version: 8}
		flag2 := ldeval.FeatureFlag{Key: "flag2", Version: 9}
		core.forceSet(ld.Features, &flag1)
		core.forceSet(ld.Features, &flag2)

		resultCh := make(chan int, 2)
		go func() {
			result, _ := w.Get(ld.Features, flag1.Key)
			resultCh <- result.GetVersion()
		}()
		<-core.queryStartedCh
		go func() {
			result, _ := w.Get(ld.Features, flag2.Key)
			resultCh <- result.GetVersion()
		}()

		results := map[int]bool{}
		results[<-resultCh] = true
		results[<-resultCh] = true
		assert.Equal(t, map[int]bool{flag1.Version: true, flag2.Version: true}, results)

		assert.Equal(t, 1, len(core.queryStartedCh)) // core received a total of 2 queries
	})

	t.Run("Cached All coalesces requests", func(t *testing.T) {
		core := newCoreWithInstrumentedQueries(cacheTime)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flag := ldeval.FeatureFlag{Key: "flag", Version: 9}
		core.forceSet(ld.Features, &flag)

		resultCh := make(chan int, 2)
		go func() {
			result, _ := w.All(ld.Features)
			resultCh <- len(result)
		}()
		<-core.queryStartedCh
		go func() {
			result, _ := w.All(ld.Features)
			resultCh <- len(result)
		}()

		result1 := <-resultCh
		result2 := <-resultCh
		assert.Equal(t, 1, result1)
		assert.Equal(t, 1, result2)

		assert.Equal(t, 0, len(core.queryStartedCh)) // core only received 1 query
	})

	t.Run("Cached store with finite TTL won't update cache if core update fails", func(t *testing.T) {
		core := newCore(cacheTime)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ldeval.FeatureFlag{Key: "flag", Version: 2}
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flagv1.Key: &flagv1},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, core.data, allData)

		core.fakeError = errors.New("sorry")
		err = w.Upsert(ld.Features, &flagv2)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		item, err := w.Get(ld.Features, flagv2.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item) // cache still has old item, same as underlying store
	})

	t.Run("Cached store with infinite TTL will update cache even if core update fails", func(t *testing.T) {
		core := newCore(-1)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ldeval.FeatureFlag{Key: "flag", Version: 2}
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flagv1.Key: &flagv1},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, core.data, allData)

		core.fakeError = errors.New("sorry")
		err = w.Upsert(ld.Features, &flagv2)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		item, err := w.Get(ld.Features, flagv2.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv2, item) // underlying store has old item but cache has new item
	})

	t.Run("Cached store with finite TTL won't update cache if core init fails", func(t *testing.T) {
		core := newCore(cacheTime)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flagv1.Key: &flagv1},
		}
		core.fakeError = errors.New("sorry")
		err := w.Init(allData)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		data, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, map[string]interfaces.VersionedData{}, data)
	})

	t.Run("Cached store with infinite TTL will update cache even if core init fails", func(t *testing.T) {
		core := newCore(-1)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flagv1 := ldeval.FeatureFlag{Key: "flag", Version: 1}
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flagv1.Key: &flagv1},
		}
		core.fakeError = errors.New("sorry")
		err := w.Init(allData)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		data, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, allData[ld.Features], data)
	})

	t.Run("Cached store with finite TTL removes cached All data if a single item is updated", func(t *testing.T) {
		core := newCore(cacheTime)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flag1v1 := ldeval.FeatureFlag{Key: "flag1", Version: 1}
		flag1v2 := ldeval.FeatureFlag{Key: "flag1", Version: 2}
		flag2v1 := ldeval.FeatureFlag{Key: "flag2", Version: 1}
		flag2v2 := ldeval.FeatureFlag{Key: "flag2", Version: 2}
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flag1v1.Key: &flag1v1, flag2v1.Key: &flag2v1},
		}
		err := w.Init(allData)
		require.NoError(t, err)

		data, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, allData[ld.Features], data)
		// now the All data is cached

		// do an Upsert for flag1 - this should drop the previous All data from the cache
		err = w.Upsert(ld.Features, &flag1v2)

		// modify flag2 directly in the underlying data
		core.forceSet(ld.Features, &flag2v2)

		// now, All should reread the underlying data so we see both changes
		data, err = w.All(ld.Features)
		require.NoError(t, err)
		assert.Equal(t, &flag1v2, allData[ld.Features][flag1v1.Key])
		assert.Equal(t, &flag2v2, allData[ld.Features][flag2v1.Key])
	})

	t.Run("Cached store with infinite TTL updates cached All data if a single item is updated", func(t *testing.T) {
		core := newCore(-1)
		w := NewDataStoreWrapperWithConfig(core, configWithoutLogging)
		defer w.Close()

		flag1v1 := ldeval.FeatureFlag{Key: "flag1", Version: 1}
		flag1v2 := ldeval.FeatureFlag{Key: "flag1", Version: 2}
		flag2v1 := ldeval.FeatureFlag{Key: "flag2", Version: 1}
		flag2v2 := ldeval.FeatureFlag{Key: "flag2", Version: 2}
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			ld.Features: {flag1v1.Key: &flag1v1, flag2v1.Key: &flag2v1},
		}
		err := w.Init(allData)
		require.NoError(t, err)

		data, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, allData[ld.Features], data)
		// now the All data is cached

		// do an Upsert for flag1 - this should update the underlying data *and* the cached All data
		err = w.Upsert(ld.Features, &flag1v2)

		// modify flag2 directly in the underlying data
		core.forceSet(ld.Features, &flag2v2)

		// now, All should *not* reread the underlying data - we should only see the change to flag1
		data, err = w.All(ld.Features)
		require.NoError(t, err)
		assert.Equal(t, &flag1v2, data[flag1v1.Key])
		assert.Equal(t, &flag2v1, data[flag2v1.Key])
	})

	t.Run("Non-atomic init passes ordered data to core", func(t *testing.T) {
		core := &mockNonAtomicCore{}
		w := NewNonAtomicDataStoreWrapper(core)

		assert.NoError(t, w.Init(dependencyOrderingTestData))

		receivedData := core.data
		assert.Equal(t, 2, len(receivedData))
		assert.Equal(t, ld.Segments, receivedData[0].Kind) // Segments should always be first
		assert.Equal(t, len(dependencyOrderingTestData[ld.Segments]), len(receivedData[0].Items))
		assert.Equal(t, ld.Features, receivedData[1].Kind)
		assert.Equal(t, len(dependencyOrderingTestData[ld.Features]), len(receivedData[1].Items))

		flags := receivedData[1].Items
		findFlagIndex := func(key string) int {
			for i, item := range flags {
				if item.GetKey() == key {
					return i
				}
			}
			return -1
		}

		for _, item := range dependencyOrderingTestData[ld.Features] {
			if flag, ok := item.(*ldeval.FeatureFlag); ok {
				flagIndex := findFlagIndex(flag.Key)
				for _, prereq := range flag.Prerequisites {
					prereqIndex := findFlagIndex(prereq.Key)
					if prereqIndex > flagIndex {
						keys := make([]string, 0, len(flags))
						for _, item := range flags {
							keys = append(keys, item.GetKey())
						}
						assert.True(t, false, "%s depends on %s, but %s was listed first; keys in order are [%s]",
							flag.Key, prereq.Key, strings.Join(keys, ", "))
					}
				}
			}
		}
	})
}

var dependencyOrderingTestData = map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
	ld.Features: {
		"a": parseFlag(`{"key":"a","prerequisites":[{"key":"b"},{"key":"c"}]}`),
		"b": parseFlag(`{"key":"b","prerequisites":[{"key":"c"},{"key":"e"}]}`),
		"c": parseFlag(`{"key":"c"}`),
		"d": parseFlag(`{"key":"d"}`),
		"e": parseFlag(`{"key":"e"}`),
		"f": parseFlag(`{"key":"f"}`),
	},
	ld.Segments: {
		"1": &ldeval.Segment{Key: "1"},
	},
}

func parseFlag(jsonString string) *ldeval.FeatureFlag {
	var f ldeval.FeatureFlag
	if err := json.Unmarshal([]byte(jsonString), &f); err != nil {
		panic(err)
	}
	return &f
}

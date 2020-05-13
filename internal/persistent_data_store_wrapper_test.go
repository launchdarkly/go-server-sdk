package internal

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

type testCacheMode string

const (
	testUncached           testCacheMode = "uncached"
	testCached             testCacheMode = "cached"
	testCachedIndefinitely testCacheMode = "cached indefinitely"
)

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
	orderedInitData  []interfaces.StoreCollection
	fakeError        error
	fakeAvailability bool
	inited           bool
	initQueriedCount int
	queryCount       int
	queryDelay       time.Duration
	queryStartedCh   chan struct{}
	lock             sync.Mutex
}

func newCore() *mockCore {
	return &mockCore{
		data:             map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{interfaces.DataKindFeatures(): {}, interfaces.DataKindSegments(): {}},
		fakeAvailability: true,
	}
}

func (c *mockCore) EnableInstrumentedQueries(queryDelay time.Duration) <-chan struct{} {
	c.queryDelay = queryDelay
	c.queryStartedCh = make(chan struct{}, 10)
	return c.queryStartedCh
}

func (c *mockCore) forceSet(kind interfaces.VersionedDataKind, item interfaces.VersionedData) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.data[kind][item.GetKey()] = item
}

func (c *mockCore) forceRemove(kind interfaces.VersionedDataKind, key string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.data[kind], key)
}

func (c *mockCore) setAvailable(available bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.fakeAvailability = available
}

func (c *mockCore) startQuery() {
	if c.queryDelay > 0 {
		c.queryStartedCh <- struct{}{}
		<-time.After(c.queryDelay)
	}
}

func (c *mockCore) Init(allData []interfaces.StoreCollection) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.fakeError != nil {
		return c.fakeError
	}
	c.orderedInitData = allData
	for k := range c.data {
		delete(c.data, k)
	}
	for _, coll := range allData {
		c.data[coll.Kind] = make(map[string]interfaces.VersionedData)
		for _, item := range coll.Items {
			c.data[coll.Kind][item.GetKey()] = item
		}
	}
	c.inited = true
	return nil
}

func (c *mockCore) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	c.startQuery()
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	return c.data[kind][key], nil
}

func (c *mockCore) GetAll(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	c.startQuery()
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	return c.data[kind], nil
}

func (c *mockCore) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) (interfaces.VersionedData, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
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

func (c *mockCore) IsInitialized() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.initQueriedCount++
	return c.inited
}

func (c *mockCore) IsStoreAvailable() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.fakeAvailability
}

func (c *mockCore) Close() error {
	return nil
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
					test(t, mode, newCore())
				})
			}
		})
	}

	runTests(t, "Get", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()
		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		flagv2 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(2).Build()

		core.forceSet(interfaces.DataKindFeatures(), &flagv1)
		item, err := w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item)

		core.forceSet(interfaces.DataKindFeatures(), &flagv2)
		item, err = w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Equal(t, &flagv1, item) // returns cached value, does not call getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Get with deleted item", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()
		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Deleted(true).Build()
		flagv2 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(2).Build()

		core.forceSet(interfaces.DataKindFeatures(), &flagv1)
		item, err := w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		require.Nil(t, item) // item is filtered out because Deleted is true

		core.forceSet(interfaces.DataKindFeatures(), &flagv2)
		item, err = w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Nil(t, item) // it used the cached deleted item rather than calling the getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Get with missing item", func(t *testing.T, mode testCacheMode, core *mockCore) {
		mockLog := sharedtest.NewMockLoggers()
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), mockLog.Loggers)
		defer w.Close()
		flag := ldbuilders.NewFlagBuilder("flag").Version(1).Build()

		item, err := w.Get(interfaces.DataKindFeatures(), flag.Key)
		require.NoError(t, err)
		require.Nil(t, item)

		assert.Nil(t, mockLog.Output[ldlog.Error]) // missing item should *not* be logged as an error by this component

		core.forceSet(interfaces.DataKindFeatures(), &flag)
		item, err = w.Get(interfaces.DataKindFeatures(), flag.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Nil(t, item) // the cache retains a nil result
		} else {
			require.Equal(t, &flag, item) // no caching, calls getter
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "cached Get uses values from Init", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()

		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		flagv2 := ldbuilders.NewFlagBuilder("flag").Version(2).Build()

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flagv1.Key: &flagv1},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, core.data, allData)

		core.forceSet(interfaces.DataKindFeatures(), &flagv2)
		item, err := w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item) // it used the cached item rather than calling the getter
	}, testCached, testCachedIndefinitely)

	runTests(t, "All", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()
		flag1 := ldbuilders.NewFlagBuilder("flag1").Version(1).Build()
		flag2 := ldbuilders.NewFlagBuilder("flag2").Version(1).Build()

		core.forceSet(interfaces.DataKindFeatures(), &flag1)
		core.forceSet(interfaces.DataKindFeatures(), &flag2)
		items, err := w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		require.Equal(t, 2, len(items))

		core.forceRemove(interfaces.DataKindFeatures(), flag2.Key)
		items, err = w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		if mode.isCached() {
			require.Equal(t, 2, len(items))
		} else {
			require.Equal(t, 1, len(items))
		}
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "cached All uses values from Init", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()

		flag1 := ldbuilders.NewFlagBuilder("flag1").Version(1).Build()
		flag2 := ldbuilders.NewFlagBuilder("flag2").Version(1).Build()

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flag1.Key: &flag1, flag2.Key: &flag2},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, allData, core.data)

		core.forceRemove(interfaces.DataKindFeatures(), flag2.Key)
		items, err := w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		require.Equal(t, 2, len(items))
	}, testCached, testCachedIndefinitely)

	runTests(t, "cached All uses fresh values if there has been an update", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()

		flag1 := ldbuilders.NewFlagBuilder("flag1").Version(1).Build()
		flag1v2 := ldbuilders.NewFlagBuilder("flag1").Version(2).Build()
		flag2 := ldbuilders.NewFlagBuilder("flag2").Version(1).Build()
		flag2v2 := ldbuilders.NewFlagBuilder("flag2").Version(2).Build()

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flag1.Key: &flag1, flag2.Key: &flag2},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, allData, core.data)

		// make a change to flag1 using the wrapper - this should flush the cache
		err = w.Upsert(interfaces.DataKindFeatures(), &flag1v2)
		require.NoError(t, err)

		// make a change to flag2 that bypasses the cache
		core.forceSet(interfaces.DataKindFeatures(), &flag2v2)

		// we should now see both changes since the cache was flushed
		items, err := w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		require.Equal(t, 2, items[flag2.Key].GetVersion())
	}, testCached)

	runTests(t, "Upsert - successful", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()

		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		flagv2 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(2).Build()

		err := w.Upsert(interfaces.DataKindFeatures(), &flagv1)
		require.NoError(t, err)
		require.Equal(t, &flagv1, core.data[interfaces.DataKindFeatures()][flagv1.Key])

		err = w.Upsert(interfaces.DataKindFeatures(), &flagv2)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[interfaces.DataKindFeatures()][flagv1.Key])

		// if we have a cache, verify that the new item is now cached by writing a different value
		// to the underlying data - Get should still return the cached item
		if mode.isCached() {
			flagv3 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(3).Build()
			core.forceSet(interfaces.DataKindFeatures(), &flagv3)
		}

		item, err := w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv2, item)
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "cached Upsert - unsuccessful", func(t *testing.T, mode testCacheMode, core *mockCore) {
		// This is for an upsert where the data in the store has a higher version. In an uncached
		// store, this is just a no-op as far as the wrapper is concerned so there's nothing to
		// test here. In a cached store, we need to verify that the cache has been refreshed
		// using the data that was found in the store.
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()

		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		flagv2 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(2).Build()

		err := w.Upsert(interfaces.DataKindFeatures(), &flagv2)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[interfaces.DataKindFeatures()][flagv1.Key])

		err = w.Upsert(interfaces.DataKindFeatures(), &flagv1)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[interfaces.DataKindFeatures()][flagv1.Key]) // value in store remains the same

		flagv3 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(3).Build()
		core.forceSet(interfaces.DataKindFeatures(), &flagv3) // bypasses cache so we can verify that flagv2 is in the cache

		item, err := w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv2, item)
	}, testCached, testCachedIndefinitely)

	runTests(t, "Delete", func(t *testing.T, mode testCacheMode, core *mockCore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()

		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		flagv2 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(2).Deleted(true).Build()
		flagv3 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(3).Build()

		core.forceSet(interfaces.DataKindFeatures(), &flagv1)
		item, err := w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item)

		err = w.Delete(interfaces.DataKindFeatures(), flagv1.Key, 2)
		require.NoError(t, err)
		require.Equal(t, &flagv2, core.data[interfaces.DataKindFeatures()][flagv1.Key])

		// make a change to the flag that bypasses the cache
		core.forceSet(interfaces.DataKindFeatures(), &flagv3)

		item, err = w.Get(interfaces.DataKindFeatures(), flagv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Nil(t, item)
		} else {
			require.Equal(t, &flagv3, item)
		}
	}, testUncached, testCached, testCachedIndefinitely)

	t.Run("Initialized calls InitializedInternal only if not already inited", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, 0, ldlog.NewDisabledLoggers())
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
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, 0, ldlog.NewDisabledLoggers())
		defer w.Close()

		assert.False(t, w.Initialized())
		assert.Equal(t, 1, core.initQueriedCount)

		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{interfaces.DataKindFeatures(): {}}
		err := w.Init(allData)
		require.NoError(t, err)

		assert.True(t, w.Initialized())
		assert.Equal(t, 1, core.initQueriedCount)
	})

	t.Run("Initialized can cache false result", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, 500*time.Millisecond, ldlog.NewDisabledLoggers())
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
		core := newCore()
		queryStartedCh := core.EnableInstrumentedQueries(200 * time.Millisecond)
		w := NewPersistentDataStoreWrapper(core, cacheTime, ldlog.NewDisabledLoggers())
		defer w.Close()

		flag := ldbuilders.NewFlagBuilder("flag").Version(9).Build()
		core.forceSet(interfaces.DataKindFeatures(), &flag)

		resultCh := make(chan int, 2)
		go func() {
			result, _ := w.Get(interfaces.DataKindFeatures(), flag.Key)
			resultCh <- result.GetVersion()
		}()
		// We can't actually *guarantee* that our second query will start while the first one is still
		// in progress, but the combination of waiting on queryStartedCh and the built-in delay in
		// mockCoreWithInstrumentedQueries should make it extremely likely.
		<-queryStartedCh
		go func() {
			result, _ := w.Get(interfaces.DataKindFeatures(), flag.Key)
			resultCh <- result.GetVersion()
		}()

		result1 := <-resultCh
		result2 := <-resultCh
		assert.Equal(t, flag.Version, result1)
		assert.Equal(t, flag.Version, result2)

		assert.Len(t, queryStartedCh, 0) // core only received 1 query
	})

	t.Run("Cached Get doesn't coalesce requests for same key", func(t *testing.T) {
		core := newCore()
		queryStartedCh := core.EnableInstrumentedQueries(200 * time.Millisecond)
		w := NewPersistentDataStoreWrapper(core, cacheTime, ldlog.NewDisabledLoggers())
		defer w.Close()

		flag1 := ldbuilders.NewFlagBuilder("flag1").Version(8).Build()
		flag2 := ldbuilders.NewFlagBuilder("flag2").Version(9).Build()
		core.forceSet(interfaces.DataKindFeatures(), &flag1)
		core.forceSet(interfaces.DataKindFeatures(), &flag2)

		resultCh := make(chan int, 2)
		go func() {
			result, _ := w.Get(interfaces.DataKindFeatures(), flag1.Key)
			resultCh <- result.GetVersion()
		}()
		<-queryStartedCh
		go func() {
			result, _ := w.Get(interfaces.DataKindFeatures(), flag2.Key)
			resultCh <- result.GetVersion()
		}()

		results := map[int]bool{}
		results[<-resultCh] = true
		results[<-resultCh] = true
		assert.Equal(t, map[int]bool{flag1.Version: true, flag2.Version: true}, results)

		assert.Len(t, core.queryStartedCh, 1) // core received a total of 2 queries
	})

	t.Run("Cached All coalesces requests", func(t *testing.T) {
		core := newCore()
		queryStartedCh := core.EnableInstrumentedQueries(200 * time.Millisecond)
		w := NewPersistentDataStoreWrapper(core, cacheTime, ldlog.NewDisabledLoggers())
		defer w.Close()

		flag := ldbuilders.NewFlagBuilder("flag").Version(9).Build()
		core.forceSet(interfaces.DataKindFeatures(), &flag)

		resultCh := make(chan int, 2)
		go func() {
			result, _ := w.All(interfaces.DataKindFeatures())
			resultCh <- len(result)
		}()
		<-queryStartedCh
		go func() {
			result, _ := w.All(interfaces.DataKindFeatures())
			resultCh <- len(result)
		}()

		result1 := <-resultCh
		result2 := <-resultCh
		assert.Equal(t, 1, result1)
		assert.Equal(t, 1, result2)

		assert.Len(t, core.queryStartedCh, 0) // core only received 1 query
	})

	t.Run("Cached store with finite TTL won't update cache if core update fails", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, cacheTime, ldlog.NewDisabledLoggers())
		defer w.Close()

		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		flagv2 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(2).Build()
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flagv1.Key: &flagv1},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, core.data, allData)

		core.fakeError = errors.New("sorry")
		err = w.Upsert(interfaces.DataKindFeatures(), &flagv2)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		item, err := w.Get(interfaces.DataKindFeatures(), flagv2.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item) // cache still has old item, same as underlying store
	})

	t.Run("Cached store with infinite TTL will update cache even if core update fails", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, -1, ldlog.NewDisabledLoggers())
		defer w.Close()

		flagv1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		flagv2 := ldbuilders.NewFlagBuilder(flagv1.Key).Version(2).Build()
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flagv1.Key: &flagv1},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, core.data, allData)

		core.fakeError = errors.New("sorry")
		err = w.Upsert(interfaces.DataKindFeatures(), &flagv2)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		item, err := w.Get(interfaces.DataKindFeatures(), flagv2.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv2, item) // underlying store has old item but cache has new item
	})

	t.Run("Cached store with finite TTL won't update cache if core init fails", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, cacheTime, ldlog.NewDisabledLoggers())
		defer w.Close()

		flag := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flag.Key: &flag},
		}
		core.fakeError = errors.New("sorry")
		err := w.Init(allData)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		data, err := w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		require.Equal(t, map[string]interfaces.VersionedData{}, data)
	})

	t.Run("Cached store with infinite TTL will update cache even if core init fails", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, -1, ldlog.NewDisabledLoggers())
		defer w.Close()

		flag := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flag.Key: &flag},
		}
		core.fakeError = errors.New("sorry")
		err := w.Init(allData)
		require.Equal(t, core.fakeError, err)

		core.fakeError = nil
		data, err := w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		require.Equal(t, allData[interfaces.DataKindFeatures()], data)
	})

	t.Run("Cached store with finite TTL removes cached All data if a single item is updated", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, cacheTime, ldlog.NewDisabledLoggers())
		defer w.Close()

		flag1v1 := ldbuilders.NewFlagBuilder("flag1").Version(1).Build()
		flag1v2 := ldbuilders.NewFlagBuilder(flag1v1.Key).Version(2).Build()
		flag2v1 := ldbuilders.NewFlagBuilder("flag2").Version(1).Build()
		flag2v2 := ldbuilders.NewFlagBuilder(flag2v1.Key).Version(2).Build()
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flag1v1.Key: &flag1v1, flag2v1.Key: &flag2v1},
		}
		err := w.Init(allData)
		require.NoError(t, err)

		data, err := w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		require.Equal(t, allData[interfaces.DataKindFeatures()], data)
		// now the All data is cached

		// do an Upsert for flag1 - this should drop the previous All data from the cache
		err = w.Upsert(interfaces.DataKindFeatures(), &flag1v2)

		// modify flag2 directly in the underlying data
		core.forceSet(interfaces.DataKindFeatures(), &flag2v2)

		// now, All should reread the underlying data so we see both changes
		data, err = w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		assert.Equal(t, &flag1v2, data[flag1v1.Key])
		assert.Equal(t, &flag2v2, data[flag2v1.Key])
	})

	t.Run("Cached store with infinite TTL updates cached All data if a single item is updated", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, -1, ldlog.NewDisabledLoggers())
		defer w.Close()

		flag1v1 := ldbuilders.NewFlagBuilder("flag1").Version(1).Build()
		flag1v2 := ldbuilders.NewFlagBuilder(flag1v1.Key).Version(2).Build()
		flag2v1 := ldbuilders.NewFlagBuilder("flag2").Version(1).Build()
		flag2v2 := ldbuilders.NewFlagBuilder(flag2v1.Key).Version(2).Build()
		allData := map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{
			interfaces.DataKindFeatures(): {flag1v1.Key: &flag1v1, flag2v1.Key: &flag2v1},
		}
		err := w.Init(allData)
		require.NoError(t, err)

		data, err := w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		require.Equal(t, allData[interfaces.DataKindFeatures()], data)
		// now the All data is cached

		// do an Upsert for flag1 - this should update the underlying data *and* the cached All data
		err = w.Upsert(interfaces.DataKindFeatures(), &flag1v2)

		// modify flag2 directly in the underlying data
		core.forceSet(interfaces.DataKindFeatures(), &flag2v2)

		// now, All should *not* reread the underlying data - we should only see the change to flag1
		data, err = w.All(interfaces.DataKindFeatures())
		require.NoError(t, err)
		assert.Equal(t, &flag1v2, data[flag1v1.Key])
		assert.Equal(t, &flag2v1, data[flag2v1.Key])
	})

	t.Run("Non-atomic init passes ordered data to core", func(t *testing.T) {
		core := newCore()
		w := NewPersistentDataStoreWrapper(core, 0, ldlog.NewDisabledLoggers())

		assert.NoError(t, w.Init(dependencyOrderingTestData))

		receivedData := core.orderedInitData
		assert.Equal(t, 2, len(receivedData))
		assert.Equal(t, interfaces.DataKindSegments(), receivedData[0].Kind) // Segments should always be first
		assert.Equal(t, len(dependencyOrderingTestData[interfaces.DataKindSegments()]), len(receivedData[0].Items))
		assert.Equal(t, interfaces.DataKindFeatures(), receivedData[1].Kind)
		assert.Equal(t, len(dependencyOrderingTestData[interfaces.DataKindFeatures()]), len(receivedData[1].Items))

		flags := receivedData[1].Items
		findFlagIndex := func(key string) int {
			for i, item := range flags {
				if item.GetKey() == key {
					return i
				}
			}
			return -1
		}

		for _, item := range dependencyOrderingTestData[interfaces.DataKindFeatures()] {
			if flag, ok := item.(*ldmodel.FeatureFlag); ok {
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
	interfaces.DataKindFeatures(): {
		"a": parseFlag(`{"key":"a","prerequisites":[{"key":"b"},{"key":"c"}]}`),
		"b": parseFlag(`{"key":"b","prerequisites":[{"key":"c"},{"key":"e"}]}`),
		"c": parseFlag(`{"key":"c"}`),
		"d": parseFlag(`{"key":"d"}`),
		"e": parseFlag(`{"key":"e"}`),
		"f": parseFlag(`{"key":"f"}`),
	},
	interfaces.DataKindSegments(): {
		"1": &ldmodel.Segment{Key: "1"},
	},
}

func parseFlag(jsonString string) *ldmodel.FeatureFlag {
	var f ldmodel.FeatureFlag
	if err := json.Unmarshal([]byte(jsonString), &f); err != nil {
		panic(err)
	}
	return &f
}

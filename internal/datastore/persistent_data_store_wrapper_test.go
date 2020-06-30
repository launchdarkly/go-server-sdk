package datastore

import (
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	st "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	s "gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
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

func (m testCacheMode) isInfiniteTTL() bool {
	return m.ttl() < 0
}

func makePersistentDataStoreWrapper(
	t *testing.T,
	mode testCacheMode,
	core *s.MockPersistentDataStore,
) intf.DataStore {
	broadcaster := internal.NewDataStoreStatusBroadcaster()
	dataStoreUpdates := NewDataStoreUpdatesImpl(broadcaster)
	return NewPersistentDataStoreWrapper(core, dataStoreUpdates, mode.ttl(), s.NewTestLoggers())
}

func TestPersistentDataStoreWrapper(t *testing.T) {
	allCacheModes := []testCacheMode{testUncached, testCached, testCachedIndefinitely}
	cachedOnly := []testCacheMode{testCached, testCachedIndefinitely}

	runTests := func(
		name string,
		test func(t *testing.T, mode testCacheMode),
		forModes ...testCacheMode,
	) {
		if len(forModes) == 0 {
			require.Fail(t, "didn't specify any testCacheModes")
		}
		t.Run(name, func(t *testing.T) {
			for _, mode := range forModes {
				t.Run(string(mode), func(t *testing.T) {
					test(t, mode)
				})
			}
		})
	}

	runTests("Get", testPersistentDataStoreWrapperGet, allCacheModes...)
	runTests("GetAll", testPersistentDataStoreWrapperGetAll, allCacheModes...)
	runTests("Upsert", testPersistentDataStoreWrapperUpsert, allCacheModes...)
	runTests("Delete", testPersistentDataStoreWrapperDelete, allCacheModes...)
	runTests("IsInitialized", testPersistentDataStoreWrapperIsInitialized, allCacheModes...)
	runTests("update failures with cache", testPersistentDataStoreWrapperUpdateFailuresWithCache, cachedOnly...)

	runTests("IsStatusMonitoringEnabled", func(t *testing.T, mode testCacheMode) {
		testWithMockPersistentDataStore(t, "is always true", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
			assert.True(t, w.IsStatusMonitoringEnabled())
		})
	}, allCacheModes...)
}

func testWithMockPersistentDataStore(
	t *testing.T,
	name string,
	mode testCacheMode,
	action func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore),
) {
	t.Run(name, func(t *testing.T) {
		core := s.NewMockPersistentDataStore()
		w := makePersistentDataStoreWrapper(t, mode, core)
		defer w.Close()
		action(t, core, w)
	})
}

func testPersistentDataStoreWrapperGet(t *testing.T, mode testCacheMode) {
	testWithMockPersistentDataStore(t, "existing item", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		itemv1 := s.MockDataItem{Key: "item", Version: 1}
		itemv2 := s.MockDataItem{Key: itemv1.Key, Version: 2}

		core.ForceSet(s.MockData, itemv1.Key, itemv1.ToSerializedItemDescriptor())
		item, err := w.Get(s.MockData, itemv1.Key)
		require.NoError(t, err)
		require.Equal(t, itemv1.ToItemDescriptor(), item)

		core.ForceSet(s.MockData, itemv1.Key, itemv2.ToSerializedItemDescriptor())
		item, err = w.Get(s.MockData, itemv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Equal(t, itemv1.ToItemDescriptor(), item) // returns cached value, does not call getter
		} else {
			require.Equal(t, itemv2.ToItemDescriptor(), item) // no caching, calls getter
		}
	})

	testWithMockPersistentDataStore(t, "unknown item", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		itemv1 := s.MockDataItem{Key: "key", Version: 1}

		item, err := w.Get(s.MockData, itemv1.Key)
		require.NoError(t, err)
		require.Equal(t, st.ItemDescriptor{}.NotFound(), item)

		core.ForceSet(s.MockData, itemv1.Key, itemv1.ToSerializedItemDescriptor())
		item, err = w.Get(s.MockData, itemv1.Key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Equal(t, st.ItemDescriptor{}.NotFound(), item) // the cache retains a nil result
		} else {
			require.Equal(t, itemv1.ToItemDescriptor(), item) // no caching, calls getter
		}
	})

	testWithMockPersistentDataStore(t, "deleted item", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		key := "item"
		deletedItemDesc := st.ItemDescriptor{Version: 1}
		serializedDeletedItemDesc := st.SerializedItemDescriptor{Version: 1}
		itemv2 := s.MockDataItem{Key: key, Version: 2}

		core.ForceSet(s.MockData, key, serializedDeletedItemDesc)
		item, err := w.Get(s.MockData, key)
		require.NoError(t, err)
		assert.Equal(t, deletedItemDesc, item)

		core.ForceSet(s.MockData, key, itemv2.ToSerializedItemDescriptor())
		item, err = w.Get(s.MockData, key)
		require.NoError(t, err)
		if mode.isCached() {
			require.Equal(t, deletedItemDesc, item) // it used the cached deleted item rather than calling the getter
		} else {
			require.Equal(t, itemv2.ToItemDescriptor(), item) // no caching, calls getter
		}
	})

	testWithMockPersistentDataStore(t, "item that fails to deserialize", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		key := "item"
		core.ForceSet(s.MockData, key, st.SerializedItemDescriptor{Version: 1, SerializedItem: []byte("BAD!")})

		_, err := w.Get(s.MockData, key)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid MockDataItem") // the error that our mock item deserializer returns
	})

	if mode.isCached() {
		t.Run("cached", func(t *testing.T) {
			testWithMockPersistentDataStore(t, "uses values from Init", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				itemv1 := s.MockDataItem{Key: "item", Version: 1}
				itemv2 := s.MockDataItem{Key: itemv1.Key, Version: 2}

				require.NoError(t, w.Init(s.MakeMockDataSet(itemv1)))

				core.ForceSet(s.MockData, itemv1.Key, itemv2.ToSerializedItemDescriptor())
				result, err := w.Get(s.MockData, itemv1.Key)
				require.NoError(t, err)
				require.Equal(t, itemv1.ToItemDescriptor(), result)
			})

			testWithMockPersistentDataStore(t, "coalesces requests for same key", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				queryStartedCh := core.EnableInstrumentedQueries(200 * time.Millisecond)

				item := s.MockDataItem{Key: "key", Version: 9}
				core.ForceSet(s.MockData, item.Key, item.ToSerializedItemDescriptor())

				resultCh := make(chan int, 2)
				go func() {
					result, _ := w.Get(s.MockData, item.Key)
					resultCh <- result.Version
				}()
				// We can't actually *guarantee* that our second query will start while the first one is still
				// in progress, but the combination of waiting on queryStartedCh and the built-in delay in
				// MockCoreWithInstrumentedQueries should make it extremely likely.
				<-queryStartedCh
				go func() {
					result, _ := w.Get(s.MockData, item.Key)
					resultCh <- result.Version
				}()

				result1 := <-resultCh
				result2 := <-resultCh
				assert.Equal(t, item.Version, result1)
				assert.Equal(t, item.Version, result2)

				assert.Len(t, queryStartedCh, 0) // core only received 1 query
			})
		})
	}

	testWithMockPersistentDataStore(t, "item whose version number doesn't come from the serialized data",
		mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
			// This is a condition that currently can't happen, but if we ever move away from always putting the
			// version number in the serialized JSON (i.e. if every persistent data store implementation has a
			// separate place to keep the version) then PersistentDataStoreWrapper should be able to handle it.
			item := s.MockDataItem{Key: "key", Version: 1}

			sid := item.ToSerializedItemDescriptor()
			sid.Version = 2

			core.ForceSet(s.MockData, item.Key, sid)

			id := item.ToItemDescriptor()
			id.Version = 2

			result, err := w.Get(s.MockData, item.Key)
			assert.NoError(t, err)
			assert.Equal(t, id, result)
		})
}

func testPersistentDataStoreWrapperGetAll(t *testing.T, mode testCacheMode) {
	testWithMockPersistentDataStore(t, "gets only items of one kind", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		item1 := s.MockDataItem{Key: "item1", Version: 1}
		item2 := s.MockDataItem{Key: "item2", Version: 1}
		otherItem1 := s.MockDataItem{Key: "item1", Version: 3, IsOtherKind: true}

		core.ForceSet(s.MockData, item1.Key, item1.ToSerializedItemDescriptor())
		core.ForceSet(s.MockData, item2.Key, item2.ToSerializedItemDescriptor())
		core.ForceSet(s.MockOtherData, otherItem1.Key, otherItem1.ToSerializedItemDescriptor())

		items, err := w.GetAll(s.MockData)
		require.NoError(t, err)
		require.Equal(t, 2, len(items))
		sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
		assert.Equal(t, []st.KeyedItemDescriptor{item1.ToKeyedItemDescriptor(), item2.ToKeyedItemDescriptor()}, items)

		items, err = w.GetAll(s.MockOtherData)
		require.NoError(t, err)
		require.Equal(t, 1, len(items))
		assert.Equal(t, []st.KeyedItemDescriptor{otherItem1.ToKeyedItemDescriptor()}, items)
	})

	testWithMockPersistentDataStore(t, "item that fails to deserialize", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		item1 := s.MockDataItem{Key: "item1", Version: 1}
		core.ForceSet(s.MockData, item1.Key, item1.ToSerializedItemDescriptor())
		core.ForceSet(s.MockData, "item2", st.SerializedItemDescriptor{Version: 1, SerializedItem: []byte("BAD!")})

		_, err := w.GetAll(s.MockData)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a valid MockDataItem") // the error that our mock item deserializer returns
	})

	if mode.isCached() {
		t.Run("cached", func(t *testing.T) {
			testWithMockPersistentDataStore(t, "uses values from Init", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				item1 := s.MockDataItem{Key: "item1", Version: 1}
				item2 := s.MockDataItem{Key: "item2", Version: 1}

				require.NoError(t, w.Init(s.MakeMockDataSet(item1, item2)))

				core.ForceRemove(s.MockData, item2.Key)

				items, err := w.GetAll(s.MockData)
				require.NoError(t, err)
				assert.Len(t, items, 2)
			})

			t.Run("uses fresh values if there has been an update", func(t *testing.T) {
				core := s.NewMockPersistentDataStore()
				w := makePersistentDataStoreWrapper(t, mode, core)
				defer w.Close()

				item1v1 := s.MockDataItem{Key: "item1", Version: 1}
				item1v2 := s.MockDataItem{Key: "item1", Version: 2}
				item2v1 := s.MockDataItem{Key: "item2", Version: 1}
				item2v2 := s.MockDataItem{Key: "item2", Version: 2}

				require.NoError(t, w.Init(s.MakeMockDataSet(item1v1, item2v2)))

				// make a change to item1 using the wrapper - this should flush the cache
				_, err := w.Upsert(s.MockData, item1v1.Key, item1v2.ToItemDescriptor())
				require.NoError(t, err)

				// make a change to item2 that bypasses the cache
				core.ForceSet(s.MockData, item2v1.Key, item2v2.ToSerializedItemDescriptor())

				// we should now see both changes since the cache was flushed
				items, err := w.GetAll(s.MockData)
				require.NoError(t, err)
				sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
				require.Equal(t, 2, items[1].Item.Version)
			})

			testWithMockPersistentDataStore(t, "uses values from Init", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				queryStartedCh := core.EnableInstrumentedQueries(200 * time.Millisecond)

				item := s.MockDataItem{Key: "key", Version: 9}
				core.ForceSet(s.MockData, item.Key, item.ToSerializedItemDescriptor())

				resultCh := make(chan int, 2)
				go func() {
					result, _ := w.GetAll(s.MockData)
					resultCh <- len(result)
				}()
				// We can't actually *guarantee* that our second query will start while the first one is still
				// in progress, but the combination of waiting on queryStartedCh and the built-in delay in
				// MockCoreWithInstrumentedQueries should make it extremely likely.
				<-queryStartedCh
				go func() {
					result, _ := w.GetAll(s.MockData)
					resultCh <- len(result)
				}()

				result1 := <-resultCh
				result2 := <-resultCh
				assert.Equal(t, 1, result1)
				assert.Equal(t, 1, result2)

				assert.Len(t, queryStartedCh, 0) // core only received 1 query
			})
		})
	}
}

func testPersistentDataStoreWrapperUpsert(t *testing.T, mode testCacheMode) {
	testWithMockPersistentDataStore(t, "successful", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		key := "item"
		itemv1 := s.MockDataItem{Key: key, Version: 1}
		itemv2 := s.MockDataItem{Key: key, Version: 2}

		updated, err := w.Upsert(s.MockData, key, itemv1.ToItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)
		require.Equal(t, itemv1.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key))

		updated, err = w.Upsert(s.MockData, key, itemv2.ToItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)
		require.Equal(t, itemv2.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key))

		// if we have a cache, verify that the new item is now cached by writing a different value
		// to the underlying data - Get should still return the cached item
		if mode.isCached() {
			itemv3 := s.MockDataItem{Key: key, Version: 3}
			core.ForceSet(s.MockData, key, itemv3.ToSerializedItemDescriptor())
		}

		result, err := w.Get(s.MockData, key)
		require.NoError(t, err)
		assert.Equal(t, itemv2.ToItemDescriptor(), result)
	})

	testWithMockPersistentDataStore(t, "unsuccessful - lower version", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		key := "item"
		itemv1 := s.MockDataItem{Key: key, Version: 1}
		itemv2 := s.MockDataItem{Key: key, Version: 2}

		updated, err := w.Upsert(s.MockData, key, itemv2.ToItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)
		require.Equal(t, itemv2.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key))

		// In a cached store, we need to verify that after an unsuccessful upsert it will refresh the
		// cache using the very latest data from the store - so here we'll sneak a higher-versioned
		// item directly into the store.
		itemv3 := s.MockDataItem{Key: key, Version: 3}
		core.ForceSet(s.MockData, key, itemv3.ToSerializedItemDescriptor())

		updated, err = w.Upsert(s.MockData, key, itemv1.ToItemDescriptor())
		require.NoError(t, err)
		assert.False(t, updated)

		result, err := w.Get(s.MockData, key)
		require.NoError(t, err)
		assert.Equal(t, itemv3.ToItemDescriptor(), result)
	})
}

func testPersistentDataStoreWrapperDelete(t *testing.T, mode testCacheMode) {
	testWithMockPersistentDataStore(t, "successful", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		key := "item"
		itemv1 := s.MockDataItem{Key: key, Version: 1}
		deletedv2 := st.ItemDescriptor{Version: 2}

		updated, err := w.Upsert(s.MockData, key, itemv1.ToItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)
		require.Equal(t, itemv1.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key))

		updated, err = w.Upsert(s.MockData, key, deletedv2)
		require.NoError(t, err)
		assert.True(t, updated)

		// if we have a cache, verify that the new item is now cached by writing a different value
		// to the underlying data - Get should still return the cached item
		if mode.isCached() {
			itemv3 := s.MockDataItem{Key: key, Version: 3}
			core.ForceSet(s.MockData, key, itemv3.ToSerializedItemDescriptor())
		}

		result, err := w.Get(s.MockData, itemv1.Key)
		require.NoError(t, err)
		assert.Equal(t, deletedv2, result)
	})

	testWithMockPersistentDataStore(t, "unsuccessful - lower version", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		key := "item"
		itemv2 := s.MockDataItem{Key: key, Version: 2}
		deletedv1 := st.ItemDescriptor{Version: 1}

		updated, err := w.Upsert(s.MockData, key, itemv2.ToItemDescriptor())
		require.NoError(t, err)
		assert.True(t, updated)
		require.Equal(t, itemv2.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key))

		updated, err = w.Upsert(s.MockData, key, deletedv1)
		require.NoError(t, err)
		assert.False(t, updated)

		result, err := w.Get(s.MockData, itemv2.Key)
		require.NoError(t, err)
		assert.Equal(t, itemv2.ToItemDescriptor(), result)
	})
}

func testPersistentDataStoreWrapperIsInitialized(t *testing.T, mode testCacheMode) {
	testWithMockPersistentDataStore(t, "won't call underlying IsInitialized if Init has been called", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
		assert.False(t, w.IsInitialized())
		assert.Equal(t, 1, core.InitQueriedCount)

		require.NoError(t, w.Init(s.MakeMockDataSet()))

		assert.True(t, w.IsInitialized())
		assert.Equal(t, 1, core.InitQueriedCount)
	})

	if mode.isCached() {
		testWithMockPersistentDataStore(t, "can cache true result", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
			assert.Equal(t, 0, core.InitQueriedCount)

			core.ForceSetInited(true)

			assert.True(t, w.IsInitialized())
			assert.Equal(t, 1, core.InitQueriedCount)

			core.ForceSetInited(false)

			assert.True(t, w.IsInitialized())
			assert.Equal(t, 1, core.InitQueriedCount)
		})

		testWithMockPersistentDataStore(t, "can cache false result", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
			assert.False(t, w.IsInitialized())
			assert.Equal(t, 1, core.InitQueriedCount)

			core.ForceSetInited(true)

			assert.False(t, w.IsInitialized())
			assert.Equal(t, 1, core.InitQueriedCount)
		})
	}
}

func testPersistentDataStoreWrapperUpdateFailuresWithCache(t *testing.T, mode testCacheMode) {
	if mode.isInfiniteTTL() {
		t.Run("infinite TTL", func(t *testing.T) {
			testWithMockPersistentDataStore(t, "will update cache even if core Upsert fails", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				key := "key"
				itemv1 := s.MockDataItem{Key: key, Version: 1}
				itemv2 := s.MockDataItem{Key: key, Version: 2}

				require.NoError(t, w.Init(s.MakeMockDataSet(itemv1)))
				assert.Equal(t, itemv1.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key))

				myError := errors.New("sorry")
				core.SetFakeError(myError)
				_, err := w.Upsert(s.MockData, key, itemv2.ToItemDescriptor())
				assert.Equal(t, myError, err)
				assert.Equal(t, itemv1.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key)) // underlying store still has old item

				core.SetFakeError(nil)
				item, err := w.Get(s.MockData, key)
				require.NoError(t, err)
				require.Equal(t, itemv2.ToItemDescriptor(), item) // cache has new item
			})

			testWithMockPersistentDataStore(t, "will update cache even if core Init fails", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				key := "key"
				itemv1 := s.MockDataItem{Key: key, Version: 1}

				myError := errors.New("sorry")
				core.SetFakeError(myError)
				err := w.Init(s.MakeMockDataSet(itemv1))
				assert.Equal(t, myError, err)
				assert.Equal(t, st.SerializedItemDescriptor{}.NotFound(), core.ForceGet(s.MockData, key)) // underlying store does not have data

				core.SetFakeError(nil)
				result, err := w.GetAll(s.MockData)
				require.NoError(t, err)
				require.Len(t, result, 1) // cache does have data
			})
		})
	} else {
		t.Run("finite TTL", func(t *testing.T) {
			testWithMockPersistentDataStore(t, "won't update cache if core Upsert fails", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				key := "key"
				itemv1 := s.MockDataItem{Key: key, Version: 1}
				itemv2 := s.MockDataItem{Key: key, Version: 2}

				require.NoError(t, w.Init(s.MakeMockDataSet(itemv1)))
				assert.Equal(t, itemv1.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key))

				myError := errors.New("sorry")
				core.SetFakeError(myError)
				_, err := w.Upsert(s.MockData, key, itemv2.ToItemDescriptor())
				assert.Equal(t, myError, err)
				assert.Equal(t, itemv1.ToSerializedItemDescriptor(), core.ForceGet(s.MockData, key)) // underlying store still has old item

				core.SetFakeError(nil)
				item, err := w.Get(s.MockData, key)
				require.NoError(t, err)
				require.Equal(t, itemv1.ToItemDescriptor(), item) // cache still has old item too
			})

			testWithMockPersistentDataStore(t, "won't update cache if core Init fails", mode, func(t *testing.T, core *s.MockPersistentDataStore, w intf.DataStore) {
				key := "key"
				itemv1 := s.MockDataItem{Key: key, Version: 1}

				myError := errors.New("sorry")
				core.SetFakeError(myError)
				err := w.Init(s.MakeMockDataSet(itemv1))
				assert.Equal(t, myError, err)
				assert.Equal(t, st.SerializedItemDescriptor{}.NotFound(), core.ForceGet(s.MockData, key)) // underlying store does not have data

				core.SetFakeError(nil)
				result, err := w.GetAll(s.MockData)
				require.NoError(t, err)
				require.Len(t, result, 0) // cache does not have data either
			})
		})
	}
}

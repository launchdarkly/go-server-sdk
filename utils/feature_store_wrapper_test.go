package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-client.v4"
)

// Test implementation of FeatureStoreCore
type mockCore struct {
	cacheTTL time.Duration
	data     map[ld.VersionedDataKind]map[string]ld.VersionedData
	inited   bool
}

func newCore(ttl time.Duration) *mockCore {
	return &mockCore{
		cacheTTL: ttl,
		data:     map[ld.VersionedDataKind]map[string]ld.VersionedData{ld.Features: {}, ld.Segments: {}},
	}
}

func (c *mockCore) forceSet(kind ld.VersionedDataKind, item ld.VersionedData) {
	c.data[kind][item.GetKey()] = item
}

func (c *mockCore) forceRemove(kind ld.VersionedDataKind, key string) {
	delete(c.data[kind], key)
}

func (c *mockCore) GetCacheTTL() time.Duration {
	return c.cacheTTL
}

func (c *mockCore) InitInternal(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
	c.data = allData
	c.inited = true
	return nil
}

func (c *mockCore) GetInternal(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
	return c.data[kind][key], nil
}

func (c *mockCore) GetAllInternal(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
	return c.data[kind], nil
}

func (c *mockCore) UpsertInternal(kind ld.VersionedDataKind, item ld.VersionedData) (bool, error) {
	if c.data[kind][item.GetKey()] != nil && c.data[kind][item.GetKey()].GetVersion() >= item.GetVersion() {
		return false, nil
	}
	c.data[kind][item.GetKey()] = item
	return true, nil
}

func (c *mockCore) InitializedInternal() bool {
	return c.inited
}

func TestFeatureStoreWrapper(t *testing.T) {
	cacheTime := 30 * time.Second

	runCachedAndUncachedTests := func(t *testing.T, name string, test func(t *testing.T, isCached bool, core *mockCore)) {
		t.Run(name, func(t *testing.T) {
			t.Run("uncached", func(t *testing.T) {
				test(t, false, newCore(0))
			})
			t.Run("cached", func(t *testing.T) {
				test(t, true, newCore(cacheTime))
			})
		})
	}

	runCachedAndUncachedTests(t, "Get", func(t *testing.T, isCached bool, core *mockCore) {
		w := NewFeatureStoreWrapper(core)
		flagv1 := ld.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag", Version: 2}

		core.forceSet(ld.Features, &flagv1)
		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item)

		core.forceSet(ld.Features, &flagv2)
		item, err = w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		if isCached {
			require.Equal(t, &flagv1, item) // returns cached value, does not call getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	})

	runCachedAndUncachedTests(t, "Get with deleted item", func(t *testing.T, isCached bool, core *mockCore) {
		w := NewFeatureStoreWrapper(core)
		flagv1 := ld.FeatureFlag{Key: "flag", Version: 1, Deleted: true}
		flagv2 := ld.FeatureFlag{Key: "flag", Version: 2, Deleted: false}

		core.forceSet(ld.Features, &flagv1)
		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Nil(t, item) // item is filtered out because Deleted is true

		core.forceSet(ld.Features, &flagv2)
		item, err = w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		if isCached {
			require.Nil(t, item) // it used the cached deleted item rather than calling the getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	})

	runCachedAndUncachedTests(t, "Get with missing item", func(t *testing.T, isCached bool, core *mockCore) {
		w := NewFeatureStoreWrapper(core)
		flag := ld.FeatureFlag{Key: "flag", Version: 1}

		item, err := w.Get(ld.Features, flag.Key)
		require.NoError(t, err)
		require.Nil(t, item)

		core.forceSet(ld.Features, &flag)
		item, err = w.Get(ld.Features, flag.Key)
		require.NoError(t, err)
		if isCached {
			require.Nil(t, item) // the cache retains a nil result
		} else {
			require.Equal(t, &flag, item) // no caching, calls getter
		}
	})

	t.Run("cached Get uses values from Init", func(t *testing.T) {
		core := newCore(cacheTime)
		w := NewFeatureStoreWrapper(core)

		flagv1 := ld.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag", Version: 1}

		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flagv1.Key: &flagv1},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, core.data, allData)

		core.forceSet(ld.Features, &flagv2)
		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		require.Equal(t, &flagv1, item) // it used the cached item rather than calling the getter
	})

	runCachedAndUncachedTests(t, "All", func(t *testing.T, isCached bool, core *mockCore) {
		w := NewFeatureStoreWrapper(core)
		flag1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flag2 := ld.FeatureFlag{Key: "flag2", Version: 1}

		core.forceSet(ld.Features, &flag1)
		core.forceSet(ld.Features, &flag2)
		items, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, 2, len(items))

		core.forceRemove(ld.Features, flag2.Key)
		items, err = w.All(ld.Features)
		require.NoError(t, err)
		if isCached {
			require.Equal(t, 2, len(items))
		} else {
			require.Equal(t, 1, len(items))
		}
	})

	t.Run("cached All uses values from Init", func(t *testing.T) {
		core := newCore(cacheTime)
		w := NewFeatureStoreWrapper(core)

		flag1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flag2 := ld.FeatureFlag{Key: "flag2", Version: 1}

		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flag1.Key: &flag1, flag2.Key: &flag2},
		}
		err := w.Init(allData)
		require.NoError(t, err)
		require.Equal(t, allData, core.data)

		core.forceRemove(ld.Features, flag2.Key)
		items, err := w.All(ld.Features)
		require.NoError(t, err)
		require.Equal(t, 2, len(items))
	})

	t.Run("cached All uses fresh values if there has been an update", func(t *testing.T) {
		core := newCore(cacheTime)
		w := NewFeatureStoreWrapper(core)

		flag1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flag1v2 := ld.FeatureFlag{Key: "flag1", Version: 2}
		flag2 := ld.FeatureFlag{Key: "flag2", Version: 1}
		flag2v2 := ld.FeatureFlag{Key: "flag2", Version: 2}

		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
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
	})

	runCachedAndUncachedTests(t, "Upsert", func(t *testing.T, isCached bool, core *mockCore) {
		w := NewFeatureStoreWrapper(core)

		flagv1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag1", Version: 2}

		err := w.Upsert(ld.Features, &flagv1)
		require.NoError(t, err)
		require.Equal(t, &flagv1, core.data[ld.Features][flagv1.Key])

		// make a change to the flag that bypasses the cache
		core.forceSet(ld.Features, &flagv2)

		item, err := w.Get(ld.Features, flagv1.Key)
		require.NoError(t, err)
		if isCached {
			require.Equal(t, &flagv1, item)
		} else {
			require.Equal(t, &flagv2, item)
		}
	})

	runCachedAndUncachedTests(t, "Delete", func(t *testing.T, isCached bool, core *mockCore) {
		w := NewFeatureStoreWrapper(core)

		flagv1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag1", Version: 2, Deleted: true}
		flagv3 := ld.FeatureFlag{Key: "flag1", Version: 3}

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
		if isCached {
			require.Nil(t, item)
		} else {
			require.Equal(t, &flagv3, item)
		}
	})

	t.Run("Initialized calls core's InitializedInternal", func(t *testing.T) {
		core := newCore(cacheTime)
		w := NewFeatureStoreWrapper(core)

		assert.False(t, w.Initialized())

		core.inited = true
		assert.True(t, w.Initialized())
	})
}

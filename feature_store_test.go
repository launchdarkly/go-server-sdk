package ldclient_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ld "gopkg.in/launchdarkly/go-client.v4"
	ldtest "gopkg.in/launchdarkly/go-client.v4/shared_test"
)

func makeInMemoryStore() ld.FeatureStore {
	return ld.NewInMemoryFeatureStore(nil)
}

func TestInMemoryFeatureStore(t *testing.T) {
	ldtest.RunFeatureStoreTests(t, makeInMemoryStore)
}

func TestFeatureStoreHelper(t *testing.T) {
	cacheTime := 30 * time.Second

	getFunc := func(err error, item ld.VersionedData) func(ld.VersionedDataKind, string) (ld.VersionedData, error) {
		return func(kind ld.VersionedDataKind, key string) (ld.VersionedData, error) {
			return item, err
		}
	}

	allFunc := func(err error, items ...ld.VersionedData) func(ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
		return func(kind ld.VersionedDataKind) (map[string]ld.VersionedData, error) {
			ret := make(map[string]ld.VersionedData)
			for _, item := range items {
				ret[item.GetKey()] = item
			}
			return ret, err
		}
	}

	initFunc := func(err error, wasCalled *bool) func(map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
		return func(allData map[ld.VersionedDataKind]map[string]ld.VersionedData) error {
			if wasCalled != nil {
				*wasCalled = true
			}
			return err
		}
	}

	upsertFunc := func(err error, receivedItem *ld.VersionedData) func(ld.VersionedDataKind, ld.VersionedData) error {
		return func(kind ld.VersionedDataKind, item ld.VersionedData) error {
			if receivedItem != nil {
				*receivedItem = item
			}
			return err
		}
	}

	runCachedAndUncachedTests := func(t *testing.T, name string, test func(t *testing.T, isCached bool, fsh *ld.FeatureStoreHelper)) {
		t.Run(name, func(t *testing.T) {
			t.Run("uncached", func(t *testing.T) {
				test(t, false, ld.NewFeatureStoreHelper(0))
			})
			t.Run("cached", func(t *testing.T) {
				test(t, true, ld.NewFeatureStoreHelper(cacheTime))
			})
		})
	}

	runCachedAndUncachedTests(t, "Get", func(t *testing.T, isCached bool, fsh *ld.FeatureStoreHelper) {
		flagv1 := ld.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag", Version: 2}

		item, err := fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv1))
		require.NoError(t, err)
		require.Equal(t, &flagv1, item)

		item, err = fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv2))
		require.NoError(t, err)
		if isCached {
			require.Equal(t, &flagv1, item) // returns cached value, does not call getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	})

	runCachedAndUncachedTests(t, "Get with deleted item", func(t *testing.T, isCached bool, fsh *ld.FeatureStoreHelper) {
		flagv1 := ld.FeatureFlag{Key: "flag", Version: 1, Deleted: true}
		flagv2 := ld.FeatureFlag{Key: "flag", Version: 2, Deleted: false}

		item, err := fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv1))
		require.NoError(t, err)
		require.Nil(t, item) // item is filtered out because Deleted is true

		item, err = fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv2))
		require.NoError(t, err)
		if isCached {
			require.Nil(t, item) // it used the cached deleted item rather than calling the getter
		} else {
			require.Equal(t, &flagv2, item) // no caching, calls getter
		}
	})

	runCachedAndUncachedTests(t, "Get with missing item", func(t *testing.T, isCached bool, fsh *ld.FeatureStoreHelper) {
		flag := ld.FeatureFlag{Key: "flag", Version: 1}

		item, err := fsh.Get(ld.Features, flag.Key, getFunc(nil, nil))
		require.NoError(t, err)
		require.Nil(t, item)

		item, err = fsh.Get(ld.Features, flag.Key, getFunc(nil, &flag))
		require.NoError(t, err)
		if isCached {
			require.Nil(t, item) // the cache retains a nil result
		} else {
			require.Equal(t, &flag, item) // no caching, calls getter
		}
	})

	t.Run("cached Get uses values from Init", func(t *testing.T) {
		fsh := ld.NewFeatureStoreHelper(cacheTime)

		flagv1 := ld.FeatureFlag{Key: "flag", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag", Version: 1}

		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flagv1.Key: &flagv1},
		}
		initWasCalled := false
		err := fsh.Init(allData, initFunc(nil, &initWasCalled))
		require.NoError(t, err)
		require.True(t, initWasCalled)

		item, err := fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv2))
		require.NoError(t, err)
		require.Equal(t, &flagv1, item) // it used the cached item rather than calling the getter
	})

	runCachedAndUncachedTests(t, "All", func(t *testing.T, isCached bool, fsh *ld.FeatureStoreHelper) {
		flag1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flag2 := ld.FeatureFlag{Key: "flag2", Version: 1}

		items, err := fsh.All(ld.Features, allFunc(nil, &flag1, &flag2))
		require.NoError(t, err)
		require.Equal(t, 2, len(items))

		items, err = fsh.All(ld.Features, allFunc(nil, &flag1))
		require.NoError(t, err)
		if isCached {
			require.Equal(t, 2, len(items))
		} else {
			require.Equal(t, 1, len(items))
		}
	})

	t.Run("cached All uses values from Init", func(t *testing.T) {
		fsh := ld.NewFeatureStoreHelper(cacheTime)

		flag1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flag2 := ld.FeatureFlag{Key: "flag2", Version: 1}

		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flag1.Key: &flag1, flag2.Key: &flag2},
		}
		initWasCalled := false
		err := fsh.Init(allData, initFunc(nil, &initWasCalled))
		require.NoError(t, err)
		require.True(t, initWasCalled)

		items, err := fsh.All(ld.Features, allFunc(nil, &flag1))
		require.NoError(t, err)
		require.Equal(t, 2, len(items))
	})

	t.Run("cached All uses fresh values if there has been an update", func(t *testing.T) {
		fsh := ld.NewFeatureStoreHelper(cacheTime)

		flag1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flag1v2 := ld.FeatureFlag{Key: "flag1", Version: 2}
		flag2 := ld.FeatureFlag{Key: "flag2", Version: 1}
		flag2v2 := ld.FeatureFlag{Key: "flag2", Version: 2}

		allData := map[ld.VersionedDataKind]map[string]ld.VersionedData{
			ld.Features: {flag1.Key: &flag1, flag2.Key: &flag2},
		}
		initWasCalled := false
		err := fsh.Init(allData, initFunc(nil, &initWasCalled))
		require.NoError(t, err)
		require.True(t, initWasCalled)

		err = fsh.Upsert(ld.Features, &flag1v2, upsertFunc(nil, nil))
		require.NoError(t, err)

		// updating any flag should force the cache for All to be flushed
		items, err := fsh.All(ld.Features, allFunc(nil, &flag1v2, &flag2v2))
		require.NoError(t, err)
		require.Equal(t, 2, items[flag2.Key].GetVersion())
	})

	runCachedAndUncachedTests(t, "Upsert", func(t *testing.T, isCached bool, fsh *ld.FeatureStoreHelper) {
		flagv1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag1", Version: 2}

		var receivedItem ld.VersionedData
		err := fsh.Upsert(ld.Features, &flagv1, upsertFunc(nil, &receivedItem))
		require.NoError(t, err)
		require.Equal(t, &flagv1, receivedItem)

		item, err := fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv2))
		require.NoError(t, err)
		if isCached {
			require.Equal(t, &flagv1, item)
		} else {
			require.Equal(t, &flagv2, item)
		}
	})

	runCachedAndUncachedTests(t, "Delete", func(t *testing.T, isCached bool, fsh *ld.FeatureStoreHelper) {
		flagv1 := ld.FeatureFlag{Key: "flag1", Version: 1}
		flagv2 := ld.FeatureFlag{Key: "flag1", Version: 2, Deleted: true}
		flagv3 := ld.FeatureFlag{Key: "flag1", Version: 3}

		item, err := fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv1))
		require.NoError(t, err)
		require.Equal(t, &flagv1, item)

		var receivedItem ld.VersionedData
		err = fsh.Delete(ld.Features, flagv1.Key, 2, upsertFunc(nil, &receivedItem))
		require.NoError(t, err)
		require.Equal(t, &flagv2, receivedItem)

		item, err = fsh.Get(ld.Features, flagv1.Key, getFunc(nil, &flagv3))
		require.NoError(t, err)
		if isCached {
			require.Nil(t, item)
		} else {
			require.Equal(t, &flagv3, item)
		}
	})
}

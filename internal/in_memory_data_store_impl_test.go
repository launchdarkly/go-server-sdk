package internal

import (
	"sort"
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func TestInMemoryDataStore(t *testing.T) {
	t.Run("Init", testInMemoryDataStoreInit)
	t.Run("Get", testInMemoryDataStoreGet)
	t.Run("GetAll", testInMemoryDataStoreGetAll)
	t.Run("Upsert", testInMemoryDataStoreUpsert)
	t.Run("Delete", testInMemoryDataStoreDelete)

	t.Run("IsStatusMonitoringEnabled", func(t *testing.T) {
		assert.False(t, makeInMemoryStore().IsStatusMonitoringEnabled())
	})

	t.Run("Close", func(t *testing.T) {
		assert.NoError(t, makeInMemoryStore().Close())
	})
}

func makeInMemoryStore() interfaces.DataStore {
	return NewInMemoryDataStore(sharedtest.NewTestLoggers())
}

func extractCollections(allData []interfaces.StoreCollection) [][]interfaces.StoreKeyedItemDescriptor {
	ret := [][]interfaces.StoreKeyedItemDescriptor{}
	for _, coll := range allData {
		ret = append(ret, coll.Items)
	}
	return ret
}

type dataItemCreator func(key string, version int, otherProperty bool) interfaces.StoreItemDescriptor

func forAllDataKinds(t *testing.T, test func(*testing.T, interfaces.StoreDataKind, dataItemCreator)) {
	test(t, interfaces.DataKindFeatures(), func(key string, version int, otherProperty bool) interfaces.StoreItemDescriptor {
		flag := ldbuilders.NewFlagBuilder(key).Version(version).On(otherProperty).Build()
		return interfaces.StoreItemDescriptor{Version: version, Item: &flag}
	})
	test(t, interfaces.DataKindSegments(), func(key string, version int, otherProperty bool) interfaces.StoreItemDescriptor {
		segment := ldbuilders.NewSegmentBuilder(key).Build()
		segment.Version = version // SegmentBuilder doesn't currently have a Version method
		if otherProperty {
			segment.Included = []string{"arbitrary value"}
		}
		return interfaces.StoreItemDescriptor{Version: version, Item: &segment}
	})
}

func testInMemoryDataStoreInit(t *testing.T) {
	t.Run("makes store initialized", func(t *testing.T) {
		store := makeInMemoryStore()
		allData := NewDataSetBuilder().Flags(ldbuilders.NewFlagBuilder("key").Build()).Build()

		require.NoError(t, store.Init(allData))

		assert.True(t, store.IsInitialized())
	})

	t.Run("completely replaces previous data", func(t *testing.T) {
		store := makeInMemoryStore()
		flag1 := ldbuilders.NewFlagBuilder("key1").Build()
		segment1 := ldbuilders.NewSegmentBuilder("key1").Build()
		allData1 := NewDataSetBuilder().Flags(flag1).Segments(segment1).Build()

		require.NoError(t, store.Init(allData1))

		flags, err := store.GetAll(interfaces.DataKindFeatures())
		require.NoError(t, err)
		segments, err := store.GetAll(interfaces.DataKindSegments())
		require.NoError(t, err)
		sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
		assert.Equal(t, extractCollections(allData1), [][]interfaces.StoreKeyedItemDescriptor{flags, segments})

		flag2 := ldbuilders.NewFlagBuilder("key2").Build()
		segment2 := ldbuilders.NewSegmentBuilder("key2").Build()
		allData2 := NewDataSetBuilder().Flags(flag2).Segments(segment2).Build()

		require.NoError(t, store.Init(allData2))

		flags, err = store.GetAll(interfaces.DataKindFeatures())
		require.NoError(t, err)
		segments, err = store.GetAll(interfaces.DataKindSegments())
		require.NoError(t, err)
		assert.Equal(t, extractCollections(allData2), [][]interfaces.StoreKeyedItemDescriptor{flags, segments})
	})
}

func testInMemoryDataStoreGet(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind interfaces.StoreDataKind, makeItem dataItemCreator) {
		t.Run("found", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))
			item := makeItem("key", 1, false)
			assert.NoError(t, store.Upsert(kind, "key", item))

			result, err := store.Get(kind, "key")
			assert.NoError(t, err)
			assert.Equal(t, item, result)
		})

		t.Run("not found", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			result, err := store.Get(kind, "no")
			assert.NoError(t, err)
			assert.Equal(t, interfaces.StoreItemDescriptor{}.NotFound(), result)
		})
	})
}

func testInMemoryDataStoreGetAll(t *testing.T) {
	store := makeInMemoryStore()
	require.NoError(t, store.Init(NewDataSetBuilder().Build()))

	result, err := store.GetAll(interfaces.DataKindFeatures())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 0)

	flag1 := ldbuilders.NewFlagBuilder("flag1").Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").Build()
	segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()
	require.NoError(t, store.Upsert(interfaces.DataKindFeatures(), flag1.Key, flagDescriptor(flag1)))
	require.NoError(t, store.Upsert(interfaces.DataKindFeatures(), flag2.Key, flagDescriptor(flag2)))
	require.NoError(t, store.Upsert(interfaces.DataKindSegments(), segment1.Key, segmentDescriptor(segment1)))

	flags, err := store.GetAll(interfaces.DataKindFeatures())
	require.NoError(t, err)
	segments, err := store.GetAll(interfaces.DataKindSegments())
	require.NoError(t, err)

	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	expected := extractCollections(NewDataSetBuilder().Flags(flag1, flag2).Segments(segment1).Build())
	assert.Equal(t, expected, [][]interfaces.StoreKeyedItemDescriptor{flags, segments})

	result, err = store.GetAll(unknownDataKind{})
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func testInMemoryDataStoreUpsert(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind interfaces.StoreDataKind, makeItem dataItemCreator) {
		t.Run("newer version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, "key", item1))

			item1a := makeItem("key", item1.Version+1, true)
			require.NoError(t, store.Upsert(kind, "key", item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1a, result)
		})

		t.Run("older version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, "key", item1))

			item1a := makeItem("key", item1.Version-1, true)
			require.NoError(t, store.Upsert(kind, "key", item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})

		t.Run("same version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, "key", item1))

			item1a := makeItem("key", item1.Version, true)
			require.NoError(t, store.Upsert(kind, "key", item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})
}

func testInMemoryDataStoreDelete(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind interfaces.StoreDataKind, makeItem dataItemCreator) {
		t.Run("newer version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, "key", item1))

			item1a := interfaces.StoreItemDescriptor{Version: item1.Version + 1, Item: nil}
			require.NoError(t, store.Upsert(kind, "key", item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1a, result)
		})

		t.Run("older version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, "key", item1))

			item1a := interfaces.StoreItemDescriptor{Version: item1.Version - 1, Item: nil}
			require.NoError(t, store.Upsert(kind, "key", item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})

		t.Run("same version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, "key", item1))

			item1a := interfaces.StoreItemDescriptor{Version: item1.Version, Item: nil}
			require.NoError(t, store.Upsert(kind, "key", item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})
}

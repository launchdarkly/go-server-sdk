package internal

import (
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

func TestInMemoryDataStore(t *testing.T) {
	t.Run("Init", testInMemoryDataStoreInit)
	t.Run("Get", testInMemoryDataStoreGet)
	t.Run("GetAll", testInMemoryDataStoreGetAll)
	t.Run("Upsert", testInMemoryDataStoreUpsert)
	t.Run("Delete", testInMemoryDataStoreDelete)

	t.Run("Close", func(t *testing.T) {
		assert.NoError(t, makeInMemoryStore().Close())
	})
}

func makeInMemoryStore() intf.DataStore {
	return NewInMemoryDataStore(ldlog.NewDisabledLoggers())
}

type dataItemCreator func(key string, version int, otherProperty bool) intf.VersionedData

func forAllDataKinds(t *testing.T, test func(*testing.T, intf.VersionedDataKind, dataItemCreator)) {
	t.Run("flags", func(t *testing.T) {
		test(t, intf.DataKindFeatures(), func(key string, version int, otherProperty bool) intf.VersionedData {
			flag := ldbuilders.NewFlagBuilder(key).Version(version).On(otherProperty).Build()
			return &flag
		})
	})
	t.Run("segments", func(t *testing.T) {
		test(t, intf.DataKindSegments(), func(key string, version int, otherProperty bool) intf.VersionedData {
			segment := ldbuilders.NewSegmentBuilder(key).Build()
			segment.Version = version // SegmentBuilder doesn't currently have a Version method
			if otherProperty {
				segment.Included = []string{"arbitrary value"}
			}
			return &segment
		})
	})
}

func testInMemoryDataStoreInit(t *testing.T) {
	t.Run("makes store initialized", func(t *testing.T) {
		store := makeInMemoryStore()
		allData := NewDataSetBuilder().Flags(ldbuilders.NewFlagBuilder("key").Build()).Build()

		require.NoError(t, store.Init(allData))

		assert.True(t, store.Initialized())
	})

	t.Run("completely replaces previous data", func(t *testing.T) {
		store := makeInMemoryStore()
		flag1 := ldbuilders.NewFlagBuilder("key1").Build()
		segment1 := ldbuilders.NewSegmentBuilder("key1").Build()
		allData1 := NewDataSetBuilder().Flags(flag1).Segments(segment1).Build()

		require.NoError(t, store.Init(allData1))

		flags, err := store.All(intf.DataKindFeatures())
		require.NoError(t, err)
		segments, err := store.All(intf.DataKindSegments())
		require.NoError(t, err)
		assert.Equal(t, allData1[interfaces.DataKindFeatures()], flags)
		assert.Equal(t, allData1[interfaces.DataKindSegments()], segments)

		flag2 := ldbuilders.NewFlagBuilder("key2").Build()
		segment2 := ldbuilders.NewSegmentBuilder("key2").Build()
		allData2 := NewDataSetBuilder().Flags(flag2).Segments(segment2).Build()

		require.NoError(t, store.Init(allData2))

		flags, err = store.All(intf.DataKindFeatures())
		require.NoError(t, err)
		segments, err = store.All(intf.DataKindSegments())
		require.NoError(t, err)
		assert.Equal(t, allData2[interfaces.DataKindFeatures()], flags)
		assert.Equal(t, allData2[interfaces.DataKindSegments()], segments)
	})
}

func testInMemoryDataStoreGet(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind intf.VersionedDataKind, makeItem dataItemCreator) {
		t.Run("found", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))
			item := makeItem("key", 1, false)
			assert.NoError(t, store.Upsert(kind, item))

			result, err := store.Get(kind, "key")
			assert.NoError(t, err)
			assert.Equal(t, item, result)
		})

		t.Run("not found", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			result, err := store.Get(kind, "no")
			assert.NoError(t, err)
			assert.Nil(t, result)
		})
	})
}

func testInMemoryDataStoreGetAll(t *testing.T) {
	store := makeInMemoryStore()
	require.NoError(t, store.Init(NewDataSetBuilder().Build()))

	result, err := store.All(intf.DataKindFeatures())
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result, 0)

	flag1 := ldbuilders.NewFlagBuilder("flag1").Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").Build()
	segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()
	require.NoError(t, store.Upsert(intf.DataKindFeatures(), &flag1))
	require.NoError(t, store.Upsert(intf.DataKindFeatures(), &flag2))
	require.NoError(t, store.Upsert(intf.DataKindSegments(), &segment1))

	flags, err := store.All(intf.DataKindFeatures())
	require.NoError(t, err)
	segments, err := store.All(intf.DataKindSegments())
	require.NoError(t, err)
	assert.Equal(t, map[string]intf.VersionedData{flag1.Key: &flag1, flag2.Key: &flag2}, flags)
	assert.Equal(t, map[string]intf.VersionedData{segment1.Key: &segment1}, segments)

	result, err = store.All(unknownDataKind{})
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func testInMemoryDataStoreUpsert(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind intf.VersionedDataKind, makeItem dataItemCreator) {
		t.Run("newer version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, item1))

			item1a := makeItem("key", item1.GetVersion()+1, true)
			require.NoError(t, store.Upsert(kind, item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1a, result)
		})

		t.Run("older version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, item1))

			item1a := makeItem("key", item1.GetVersion()-1, true)
			require.NoError(t, store.Upsert(kind, item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})

		t.Run("same version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, item1))

			item1a := makeItem("key", item1.GetVersion(), true)
			require.NoError(t, store.Upsert(kind, item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})
}

func testInMemoryDataStoreDelete(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind intf.VersionedDataKind, makeItem dataItemCreator) {
		t.Run("newer version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, item1))

			item1a := kind.MakeDeletedItem(item1.GetKey(), item1.GetVersion()+1)
			require.NoError(t, store.Upsert(kind, item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Nil(t, result)
		})

		t.Run("older version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, item1))

			item1a := kind.MakeDeletedItem(item1.GetKey(), item1.GetVersion()-1)
			require.NoError(t, store.Upsert(kind, item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})

		t.Run("same version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			require.NoError(t, store.Upsert(kind, item1))

			item1a := kind.MakeDeletedItem(item1.GetKey(), item1.GetVersion())
			require.NoError(t, store.Upsert(kind, item1a))

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})
}

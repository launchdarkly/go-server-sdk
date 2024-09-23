package memorystorev2

import (
	"fmt"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"sort"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryDataStore(t *testing.T) {
	t.Run("Get", testInMemoryDataStoreGet)
	t.Run("GetAll", testInMemoryDataStoreGetAll)
	t.Run("SetBasis", testInMemoryDataStoreSetBasis)
	t.Run("ApplyDelta", testInMemoryDataStoreApplyDelta)
	t.Run("Dump", testInMemoryDataStoreDump)
}

func makeMemoryStore() *Store {
	return New(sharedtest.NewTestLoggers())
}

// The dataItemCreator/forAllDataKinds helpers work for testing the FDv1-style of interacting with the memory store,
// e.g. Upsert/Init. With FDv2, the store is initialized with SetBasis and updates are applied atomically in batches
// with ApplyDelta. In order to easily inject data into the store, and then make assertions based on the result of
// calling Get, we need a slightly more involved pattern.
// The main difference is that forAllDataKindsCollection now returns the ItemDescriptor, along with a collection
// containing only that item. That way, the collection can be passed to ApplyDelta, and the ItemDescriptor can be
// used when making assertions using the result of Get.
type collectionItemCreator func(key string, version int, otherProperty bool) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection)

type collectionItemDeleter func(key string, version int) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection)

func makeCollection(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) []ldstoretypes.Collection {
	return []ldstoretypes.Collection{
		{
			Kind: kind,
			Items: []ldstoretypes.KeyedItemDescriptor{
				{
					Key:  key,
					Item: item,
				},
			},
		},
	}
}

func forAllDataKindsCollection(t *testing.T, test func(*testing.T, ldstoretypes.DataKind, collectionItemCreator, collectionItemDeleter)) {
	test(t, datakinds.Features, func(key string, version int, otherProperty bool) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		flag := ldbuilders.NewFlagBuilder(key).Version(version).On(otherProperty).Build()
		descriptor := sharedtest.FlagDescriptor(flag)

		return descriptor, makeCollection(datakinds.Features, flag.Key, descriptor)
	}, func(key string, version int) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		descriptor := ldstoretypes.ItemDescriptor{Version: version, Item: nil}

		return descriptor, makeCollection(datakinds.Features, key, descriptor)
	})
	test(t, datakinds.Segments, func(key string, version int, otherProperty bool) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		segment := ldbuilders.NewSegmentBuilder(key).Version(version).Build()
		if otherProperty {
			segment.Included = []string{"arbitrary value"}
		}
		descriptor := sharedtest.SegmentDescriptor(segment)

		return descriptor, makeCollection(datakinds.Segments, segment.Key, descriptor)
	}, func(key string, version int) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		descriptor := ldstoretypes.ItemDescriptor{Version: version, Item: nil}

		return descriptor, makeCollection(datakinds.Segments, key, descriptor)
	})
}

func testInMemoryDataStoreSetBasis(t *testing.T) {
	// SetBasis is currently an alias for Init, so the tests should be the same. Once there is no longer a use-case
	// for Init (when fdv1 data system is removed, the Init tests can be deleted.)

	t.Run("makes store initialized", func(t *testing.T) {
		store := makeMemoryStore()
		allData := sharedtest.NewDataSetBuilder().Flags(ldbuilders.NewFlagBuilder("key").Build()).Build()

		store.SetBasis(allData)

		assert.True(t, store.IsInitialized())
	})

	t.Run("completely replaces previous data", func(t *testing.T) {
		store := makeMemoryStore()
		flag1 := ldbuilders.NewFlagBuilder("key1").Build()
		segment1 := ldbuilders.NewSegmentBuilder("key1").Build()
		allData1 := sharedtest.NewDataSetBuilder().Flags(flag1).Segments(segment1).Build()

		store.SetBasis(allData1)

		flags, err := store.GetAll(datakinds.Features)
		require.NoError(t, err)
		segments, err := store.GetAll(datakinds.Segments)
		require.NoError(t, err)
		sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
		assert.Equal(t, datastore.extractCollections(allData1), [][]ldstoretypes.KeyedItemDescriptor{flags, segments})

		flag2 := ldbuilders.NewFlagBuilder("key2").Build()
		segment2 := ldbuilders.NewSegmentBuilder("key2").Build()
		allData2 := sharedtest.NewDataSetBuilder().Flags(flag2).Segments(segment2).Build()

		store.SetBasis(allData2)

		flags, err = store.GetAll(datakinds.Features)
		require.NoError(t, err)
		segments, err = store.GetAll(datakinds.Segments)
		require.NoError(t, err)
		assert.Equal(t, datastore.extractCollections(allData2), [][]ldstoretypes.KeyedItemDescriptor{flags, segments})
	})
}

func testInMemoryDataStoreGet(t *testing.T) {
	const unknownKey = "unknown-key"

	forAllDataKindsCollection(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem datastore.dataItemCreator) {
		t.Run("found", func(t *testing.T) {
			store := makeMemoryStore()
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))
			item := makeItem("key", 1, false)
			_, err := store.Upsert(kind, "key", item)
			assert.NoError(t, err)

			result, err := store.Get(kind, "key")
			assert.NoError(t, err)
			assert.Equal(t, item, result)
		})

		t.Run("not found", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			mockLog.Loggers.SetMinLevel(ldlog.Info)
			store := datastore.NewInMemoryDataStore(mockLog.Loggers)
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			result, err := store.Get(kind, unknownKey)
			assert.NoError(t, err)
			assert.Equal(t, ldstoretypes.ItemDescriptor{}.NotFound(), result)

			assert.Len(t, mockLog.GetAllOutput(), 0)
		})

		t.Run("not found - debug logging", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			mockLog.Loggers.SetMinLevel(ldlog.Debug)
			store := datastore.NewInMemoryDataStore(mockLog.Loggers)
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			result, err := store.Get(kind, unknownKey)
			assert.NoError(t, err)
			assert.Equal(t, ldstoretypes.ItemDescriptor{}.NotFound(), result)

			assert.Len(t, mockLog.GetAllOutput(), 1)
			assert.Equal(t,
				ldlogtest.MockLogItem{
					Level:   ldlog.Debug,
					Message: fmt.Sprintf(`Key %s not found in "%s"`, unknownKey, kind.GetName()),
				},
				mockLog.GetAllOutput()[0],
			)
		})
	})
}

func testInMemoryDataStoreGetAll(t *testing.T) {
	store := makeMemoryStore()
	require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

	result, err := store.GetAll(datakinds.Features)
	require.NoError(t, err)
	assert.Len(t, result, 0)

	flag1 := ldbuilders.NewFlagBuilder("flag1").Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").Build()
	segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()
	_, err = store.Upsert(datakinds.Features, flag1.Key, sharedtest.FlagDescriptor(flag1))
	require.NoError(t, err)
	_, err = store.Upsert(datakinds.Features, flag2.Key, sharedtest.FlagDescriptor(flag2))
	require.NoError(t, err)
	_, err = store.Upsert(datakinds.Segments, segment1.Key, sharedtest.SegmentDescriptor(segment1))
	require.NoError(t, err)

	flags, err := store.GetAll(datakinds.Features)
	require.NoError(t, err)
	segments, err := store.GetAll(datakinds.Segments)
	require.NoError(t, err)

	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	expected := datastore.extractCollections(sharedtest.NewDataSetBuilder().Flags(flag1, flag2).Segments(segment1).Build())
	assert.Equal(t, expected, [][]ldstoretypes.KeyedItemDescriptor{flags, segments})

	result, err = store.GetAll(datastore.unknownDataKind{})
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func testInMemoryDataStoreApplyDelta(t *testing.T) {

	forAllDataKindsCollection(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem collectionItemCreator, deleteItem collectionItemDeleter) {

		t.Run("upserts", func(t *testing.T) {

			t.Run("newer version", func(t *testing.T) {
				store := makeMemoryStore()
				store.SetBasis(sharedtest.NewDataSetBuilder().Build())

				_, collection1 := makeItem("key", 10, false)

				updates := store.ApplyDelta(collection1)
				assert.True(t, updates[kind]["key"])

				item1a, collection1a := makeItem("key", 11, true)

				updates = store.ApplyDelta(collection1a)
				assert.True(t, updates[kind]["key"])

				result, err := store.Get(kind, "key")
				require.NoError(t, err)
				assert.Equal(t, item1a, result)

			})

			t.Run("older version", func(t *testing.T) {
				store := makeMemoryStore()
				store.SetBasis(sharedtest.NewDataSetBuilder().Build())

				item1Version := 10
				item1, collection1 := makeItem("key", item1Version, false)

				updates := store.ApplyDelta(collection1)
				assert.True(t, updates[kind]["key"])

				_, collection1a := makeItem("key", item1Version-1, true)

				updates = store.ApplyDelta(collection1a)
				assert.False(t, updates[kind]["key"])

				result, err := store.Get(kind, "key")
				require.NoError(t, err)
				assert.Equal(t, item1, result)
			})

			t.Run("same version", func(t *testing.T) {
				store := makeMemoryStore()
				store.SetBasis(sharedtest.NewDataSetBuilder().Build())

				item1Version := 10
				item1, collection1 := makeItem("key", item1Version, false)
				updated := store.ApplyDelta(collection1)
				assert.True(t, updated[kind]["key"])

				_, collection1a := makeItem("key", item1Version, true)
				updated = store.ApplyDelta(collection1a)
				assert.False(t, updated[kind]["key"])

				result, err := store.Get(kind, "key")
				require.NoError(t, err)
				assert.Equal(t, item1, result)
			})
		})

		t.Run("deletes", func(t *testing.T) {
			t.Run("newer version", func(t *testing.T) {
				store := makeMemoryStore()
				require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

				item1, collection1 := makeItem("key", 10, false)
				updated := store.ApplyDelta(collection1)
				assert.True(t, updated[kind]["key"])

				item1a, collection1a := deleteItem("key", item1.Version+1)
				updated = store.ApplyDelta(collection1a)
				assert.True(t, updated[kind]["key"])

				result, err := store.Get(kind, "key")
				require.NoError(t, err)
				assert.Equal(t, item1a, result)
			})

			t.Run("older version", func(t *testing.T) {
				store := makeMemoryStore()
				require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

				item1, collection1 := makeItem("key", 10, false)
				updated := store.ApplyDelta(collection1)
				assert.True(t, updated[kind]["key"])

				_, collection1a := deleteItem("key", item1.Version-1)
				updated = store.ApplyDelta(collection1a)
				assert.False(t, updated[kind]["key"])

				result, err := store.Get(kind, "key")
				require.NoError(t, err)
				assert.Equal(t, item1, result)
			})

			t.Run("same version", func(t *testing.T) {
				store := makeMemoryStore()
				require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

				item1, collection1 := makeItem("key", 10, false)
				updated := store.ApplyDelta(collection1)
				assert.True(t, updated[kind]["key"])

				_, collection1a := deleteItem("key", item1.Version)
				updated = store.ApplyDelta(collection1a)
				assert.False(t, updated[kind]["key"])

				result, err := store.Get(kind, "key")
				require.NoError(t, err)
				assert.Equal(t, item1, result)
			})
		})
	})
}

func testInMemoryDataStoreDump(t *testing.T) {

}

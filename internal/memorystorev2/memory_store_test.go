package memorystorev2

import (
	"errors"
	"fmt"
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
	t.Run("Get", testGet)
	t.Run("GetAll", testGetAll)
	t.Run("GetAllKinds", testGetAllKinds)
	t.Run("SetBasis", testSetBasis)
	t.Run("ApplyDelta", testApplyDelta)
}

func makeMemoryStore() *Store {
	return New(sharedtest.NewTestLoggers())
}

// Used to create a segment/flag. Returns the individual item, and a collection slice
// containing only that item.
type collectionItemCreator func(key string, version int, otherProperty bool) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection)

// Used to delete a segment/flag. Returns the individual item, and a collection slice
// containing only that item.
type collectionItemDeleter func(key string, version int) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection)

func makeCollections(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) []ldstoretypes.Collection {
	return []ldstoretypes.Collection{
		makeCollection(kind, key, item),
	}
}

func makeCollection(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) ldstoretypes.Collection {
	return ldstoretypes.Collection{
		Kind: kind,
		Items: []ldstoretypes.KeyedItemDescriptor{
			{
				Key:  key,
				Item: item,
			},
		},
	}
}

func forAllDataKinds(t *testing.T, test func(*testing.T, ldstoretypes.DataKind, collectionItemCreator, collectionItemDeleter)) {
	test(t, datakinds.Features, func(key string, version int, otherProperty bool) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		flag := ldbuilders.NewFlagBuilder(key).Version(version).On(otherProperty).Build()
		descriptor := sharedtest.FlagDescriptor(flag)

		return descriptor, makeCollections(datakinds.Features, flag.Key, descriptor)
	}, func(key string, version int) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		descriptor := ldstoretypes.ItemDescriptor{Version: version, Item: nil}

		return descriptor, makeCollections(datakinds.Features, key, descriptor)
	})
	test(t, datakinds.Segments, func(key string, version int, otherProperty bool) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		segment := ldbuilders.NewSegmentBuilder(key).Version(version).Build()
		if otherProperty {
			segment.Included = []string{"arbitrary value"}
		}
		descriptor := sharedtest.SegmentDescriptor(segment)

		return descriptor, makeCollections(datakinds.Segments, segment.Key, descriptor)
	}, func(key string, version int) (ldstoretypes.ItemDescriptor, []ldstoretypes.Collection) {
		descriptor := ldstoretypes.ItemDescriptor{Version: version, Item: nil}

		return descriptor, makeCollections(datakinds.Segments, key, descriptor)
	})
}

func testSetBasis(t *testing.T) {
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
		assert.Equal(t, extractCollections(allData1), [][]ldstoretypes.KeyedItemDescriptor{flags, segments})

		flag2 := ldbuilders.NewFlagBuilder("key2").Build()
		segment2 := ldbuilders.NewSegmentBuilder("key2").Build()
		allData2 := sharedtest.NewDataSetBuilder().Flags(flag2).Segments(segment2).Build()

		store.SetBasis(allData2)

		flags, err = store.GetAll(datakinds.Features)
		require.NoError(t, err)
		segments, err = store.GetAll(datakinds.Segments)
		require.NoError(t, err)
		assert.Equal(t, extractCollections(allData2), [][]ldstoretypes.KeyedItemDescriptor{flags, segments})
	})
}

func testGet(t *testing.T) {
	const unknownKey = "unknown-key"

	forAllDataKinds(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem collectionItemCreator, _ collectionItemDeleter) {
		t.Run("found", func(t *testing.T) {
			store := makeMemoryStore()
			store.SetBasis(sharedtest.NewDataSetBuilder().Build())

			item, collection := makeItem("key", 1, false)
			store.ApplyDelta(collection)

			result, err := store.Get(kind, "key")
			assert.NoError(t, err)
			assert.Equal(t, item, result)
		})

		t.Run("not found", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			mockLog.Loggers.SetMinLevel(ldlog.Info)
			store := New(mockLog.Loggers)
			store.SetBasis(sharedtest.NewDataSetBuilder().Build())

			result, err := store.Get(kind, unknownKey)
			assert.NoError(t, err)
			assert.Equal(t, ldstoretypes.ItemDescriptor{}.NotFound(), result)

			assert.Len(t, mockLog.GetAllOutput(), 0)
		})

		t.Run("not found - debug logging", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			mockLog.Loggers.SetMinLevel(ldlog.Debug)
			store := New(mockLog.Loggers)
			store.SetBasis(sharedtest.NewDataSetBuilder().Build())

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

func testGetAll(t *testing.T) {
	store := makeMemoryStore()
	store.SetBasis(sharedtest.NewDataSetBuilder().Build())

	result, err := store.GetAll(datakinds.Features)
	require.NoError(t, err)
	assert.Len(t, result, 0)

	flag1 := ldbuilders.NewFlagBuilder("flag1").Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").Build()
	segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()

	collection := []ldstoretypes.Collection{
		{
			Kind: datakinds.Features,
			Items: []ldstoretypes.KeyedItemDescriptor{
				{
					Key:  flag1.Key,
					Item: sharedtest.FlagDescriptor(flag1),
				},
				{
					Key:  flag2.Key,
					Item: sharedtest.FlagDescriptor(flag2),
				},
			},
		},
		{
			Kind: datakinds.Segments,
			Items: []ldstoretypes.KeyedItemDescriptor{
				{
					Key:  segment1.Key,
					Item: sharedtest.SegmentDescriptor(segment1),
				},
			},
		},
	}

	store.ApplyDelta(collection)

	flags, err := store.GetAll(datakinds.Features)
	require.NoError(t, err)
	segments, err := store.GetAll(datakinds.Segments)
	require.NoError(t, err)

	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	expected := extractCollections(sharedtest.NewDataSetBuilder().Flags(flag1, flag2).Segments(segment1).Build())
	assert.Equal(t, expected, [][]ldstoretypes.KeyedItemDescriptor{flags, segments})

	result, err = store.GetAll(unknownDataKind{})
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func extractCollections(allData []ldstoretypes.Collection) [][]ldstoretypes.KeyedItemDescriptor {
	var ret [][]ldstoretypes.KeyedItemDescriptor
	for _, coll := range allData {
		ret = append(ret, coll.Items)
	}
	return ret
}

type unknownDataKind struct{}

func (k unknownDataKind) GetName() string {
	return "unknown"
}

func (k unknownDataKind) Serialize(item ldstoretypes.ItemDescriptor) []byte {
	return nil
}

func (k unknownDataKind) Deserialize(data []byte) (ldstoretypes.ItemDescriptor, error) {
	return ldstoretypes.ItemDescriptor{}, errors.New("not implemented")
}

func testApplyDelta(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem collectionItemCreator, deleteItem collectionItemDeleter) {
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
				store.SetBasis(sharedtest.NewDataSetBuilder().Build())

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
				store.SetBasis(sharedtest.NewDataSetBuilder().Build())

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
				store.SetBasis(sharedtest.NewDataSetBuilder().Build())

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

func testGetAllKinds(t *testing.T) {
	t.Run("uninitialized store", func(t *testing.T) {
		store := makeMemoryStore()
		collections := store.GetAllKinds()
		assert.Empty(t, collections)
	})

	t.Run("initialized but empty store", func(t *testing.T) {
		store := makeMemoryStore()
		store.SetBasis(sharedtest.NewDataSetBuilder().Build())

		collections := store.GetAllKinds()
		assert.Len(t, collections, 2)
		assert.Empty(t, collections[0].Items)
		assert.Empty(t, collections[1].Items)
	})

	t.Run("initialized store with data of a single kind", func(t *testing.T) {
		forAllDataKinds(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem collectionItemCreator, _ collectionItemDeleter) {
			store := makeMemoryStore()
			store.SetBasis(sharedtest.NewDataSetBuilder().Build())

			item1, collection1 := makeItem("key1", 1, false)

			store.ApplyDelta(collection1)

			collections := store.GetAllKinds()

			assert.Len(t, collections, 2)

			for _, coll := range collections {
				if coll.Kind == kind {
					assert.Len(t, coll.Items, 1)
					assert.Equal(t, item1, coll.Items[0].Item)
				} else {
					assert.Empty(t, coll.Items)
				}
			}
		})
	})

	t.Run("initialized store with data of multiple kinds", func(t *testing.T) {
		store := makeMemoryStore()
		store.SetBasis(sharedtest.NewDataSetBuilder().Build())

		flag1 := ldbuilders.NewFlagBuilder("flag1").Build()
		segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()

		expectedCollection := []ldstoretypes.Collection{
			makeCollection(datakinds.Features, flag1.Key, sharedtest.FlagDescriptor(flag1)),
			makeCollection(datakinds.Segments, segment1.Key, sharedtest.SegmentDescriptor(segment1)),
		}

		store.ApplyDelta(expectedCollection)

		gotCollections := store.GetAllKinds()

		requireCollectionsMatch(t, expectedCollection, gotCollections)
	})

	t.Run("multiple deltas applies", func(t *testing.T) {
		forAllDataKinds(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem collectionItemCreator, deleteItem collectionItemDeleter) {
			store := makeMemoryStore()

			store.SetBasis(sharedtest.NewDataSetBuilder().Build())

			_, collection1 := makeItem("key1", 1, false)
			store.ApplyDelta(collection1)

			// The collection slice we get from GetAllKinds is going to contain the specific segment or flag
			// collection we're creating here in the test, but also an empty collection for the other kind.
			expected := []ldstoretypes.Collection{collection1[0]}
			if kind == datakinds.Features {
				expected = append(expected, ldstoretypes.Collection{Kind: datakinds.Segments, Items: nil})
			} else {
				expected = append(expected, ldstoretypes.Collection{Kind: datakinds.Features, Items: nil})
			}

			requireCollectionsMatch(t, expected, store.GetAllKinds())

			_, collection1a := makeItem("key1", 2, false)
			store.ApplyDelta(collection1a)
			expected[0] = collection1a[0]
			requireCollectionsMatch(t, expected, store.GetAllKinds())

			_, collection1b := deleteItem("key1", 3)
			store.ApplyDelta(collection1b)
			expected[0] = collection1b[0]
			requireCollectionsMatch(t, expected, store.GetAllKinds())
		})
	})

	t.Run("deltas containing multiple item kinds", func(t *testing.T) {

		store := makeMemoryStore()

		store.SetBasis(sharedtest.NewDataSetBuilder().Build())

		// Flag1 will be deleted.
		flag1 := ldbuilders.NewFlagBuilder("flag1").Build()

		// Flag2 is a control and won't be changed.
		flag2 := ldbuilders.NewFlagBuilder("flag2").Build()

		// Segment1 will be upserted.
		segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()

		collection1 := []ldstoretypes.Collection{
			{
				Kind: datakinds.Features,
				Items: []ldstoretypes.KeyedItemDescriptor{
					{
						Key:  flag1.Key,
						Item: sharedtest.FlagDescriptor(flag1),
					},
					{
						Key:  flag2.Key,
						Item: sharedtest.FlagDescriptor(flag2),
					},
				},
			},
			makeCollection(datakinds.Segments, segment1.Key, sharedtest.SegmentDescriptor(segment1)),
		}

		store.ApplyDelta(collection1)

		requireCollectionsMatch(t, collection1, store.GetAllKinds())

		// Bumping the segment version is sufficient for an upsert.
		// To indicate that there's no change to flag2, we simply don't pass it in the collection.
		segment1.Version += 1
		collection2 := []ldstoretypes.Collection{
			// Delete flag1
			makeCollection(datakinds.Features, flag1.Key, ldstoretypes.ItemDescriptor{Version: flag1.Version + 1, Item: nil}),
			// Upsert segment1
			makeCollection(datakinds.Segments, segment1.Key, sharedtest.SegmentDescriptor(segment1)),
		}

		store.ApplyDelta(collection2)

		expected := []ldstoretypes.Collection{
			{
				Kind: datakinds.Features,
				Items: []ldstoretypes.KeyedItemDescriptor{
					{
						Key:  flag1.Key,
						Item: ldstoretypes.ItemDescriptor{Version: flag1.Version + 1, Item: nil},
					},
					{
						Key:  flag2.Key,
						Item: sharedtest.FlagDescriptor(flag2),
					},
				},
			},
			makeCollection(datakinds.Segments, segment1.Key, sharedtest.SegmentDescriptor(segment1)),
		}

		requireCollectionsMatch(t, expected, store.GetAllKinds())
	})
}

// Make a custom Matcher that will match the result of store.GetAllKinds() with a collection that was passed in via
// ApplyDelta or SetBasis. We need this because:
// 1) The collections (segments, features) might be in random order in the top-level slice. That is, it might be
// {segments, features} or it might be {features, segments}/
// 2) The items within each of those collections might be in random order.
// This should make use of normal assert functions where possible, and should accept a testing.T
func requireCollectionsMatch(t *testing.T, expected []ldstoretypes.Collection, actual []ldstoretypes.Collection) {
	require.Equal(t, len(expected), len(actual))
	for _, expectedCollection := range expected {
		for _, actualCollection := range actual {
			if expectedCollection.Kind == actualCollection.Kind {
				require.ElementsMatch(t, expectedCollection.Items, actualCollection.Items)
				break
			}
		}
	}
}

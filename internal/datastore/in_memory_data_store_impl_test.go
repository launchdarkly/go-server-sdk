package datastore

import (
	"fmt"
	"sort"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func makeInMemoryStore() subsystems.DataStore {
	return NewInMemoryDataStore(sharedtest.NewTestLoggers())
}

func extractCollections(allData []ldstoretypes.Collection) [][]ldstoretypes.KeyedItemDescriptor {
	ret := [][]ldstoretypes.KeyedItemDescriptor{}
	for _, coll := range allData {
		ret = append(ret, coll.Items)
	}
	return ret
}

type dataItemCreator func(key string, version int, otherProperty bool) ldstoretypes.ItemDescriptor

func forAllDataKinds(t *testing.T, test func(*testing.T, ldstoretypes.DataKind, dataItemCreator)) {
	test(t, datakinds.Features, func(key string, version int, otherProperty bool) ldstoretypes.ItemDescriptor {
		flag := ldbuilders.NewFlagBuilder(key).Version(version).On(otherProperty).Build()
		return sharedtest.FlagDescriptor(flag)
	})
	test(t, datakinds.Segments, func(key string, version int, otherProperty bool) ldstoretypes.ItemDescriptor {
		segment := ldbuilders.NewSegmentBuilder(key).Version(version).Build()
		if otherProperty {
			segment.Included = []string{"arbitrary value"}
		}
		return sharedtest.SegmentDescriptor(segment)
	})
	test(t, datakinds.ConfigOverrides, func(key string, version int, otherProperty bool) ldstoretypes.ItemDescriptor {
		override := ldbuilders.NewConfigOverrideBuilder(key).Version(version).Build()
		if otherProperty {
			override.Value = ldvalue.String("arbitrary value")
		}
		return sharedtest.ConfigOverrideDescriptor(override)
	})
	test(t, datakinds.Metrics, func(key string, version int, otherProperty bool) ldstoretypes.ItemDescriptor {
		metric := ldbuilders.NewMetricBuilder(key).Version(version).Build()
		if otherProperty {
			metric.SamplingRatio = ldvalue.NewOptionalInt(10)
		}
		return sharedtest.MetricDescriptor(metric)
	})
}

func testInMemoryDataStoreInit(t *testing.T) {
	t.Run("makes store initialized", func(t *testing.T) {
		store := makeInMemoryStore()
		allData := sharedtest.NewDataSetBuilder().Flags(ldbuilders.NewFlagBuilder("key").Build()).Build()

		require.NoError(t, store.Init(allData))

		assert.True(t, store.IsInitialized())
	})

	t.Run("completely replaces previous data", func(t *testing.T) {
		store := makeInMemoryStore()
		flag1 := ldbuilders.NewFlagBuilder("key1").Build()
		segment1 := ldbuilders.NewSegmentBuilder("key1").Build()
		configOverride1 := ldbuilders.NewConfigOverrideBuilder("key1").Build()
		metric1 := ldbuilders.NewMetricBuilder("key1").Build()
		allData1 := sharedtest.NewDataSetBuilder().Flags(flag1).Segments(segment1).ConfigOverrides(configOverride1).Metrics(metric1).Build()

		require.NoError(t, store.Init(allData1))

		flags, err := store.GetAll(datakinds.Features)
		require.NoError(t, err)
		segments, err := store.GetAll(datakinds.Segments)
		require.NoError(t, err)
		overrides, err := store.GetAll(datakinds.ConfigOverrides)
		require.NoError(t, err)
		metrics, err := store.GetAll(datakinds.Metrics)
		require.NoError(t, err)
		sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
		assert.Equal(t, extractCollections(allData1), [][]ldstoretypes.KeyedItemDescriptor{flags, segments, overrides, metrics})

		flag2 := ldbuilders.NewFlagBuilder("key2").Build()
		segment2 := ldbuilders.NewSegmentBuilder("key2").Build()
		configOverride2 := ldbuilders.NewConfigOverrideBuilder("key2").Build()
		metric2 := ldbuilders.NewMetricBuilder("key2").Build()
		allData2 := sharedtest.NewDataSetBuilder().Flags(flag2).Segments(segment2).ConfigOverrides(configOverride2).Metrics(metric2).Build()

		require.NoError(t, store.Init(allData2))

		flags, err = store.GetAll(datakinds.Features)
		require.NoError(t, err)
		segments, err = store.GetAll(datakinds.Segments)
		require.NoError(t, err)
		overrides, err = store.GetAll(datakinds.ConfigOverrides)
		require.NoError(t, err)
		metrics, err = store.GetAll(datakinds.Metrics)
		require.NoError(t, err)
		assert.Equal(t, extractCollections(allData2), [][]ldstoretypes.KeyedItemDescriptor{flags, segments, overrides, metrics})
	})
}

func testInMemoryDataStoreGet(t *testing.T) {
	const unknownKey = "unknown-key"

	forAllDataKinds(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem dataItemCreator) {
		t.Run("found", func(t *testing.T) {
			store := makeInMemoryStore()
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
			store := NewInMemoryDataStore(mockLog.Loggers)
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			result, err := store.Get(kind, unknownKey)
			assert.NoError(t, err)
			assert.Equal(t, ldstoretypes.ItemDescriptor{}.NotFound(), result)

			assert.Len(t, mockLog.GetAllOutput(), 0)
		})

		t.Run("not found - debug logging", func(t *testing.T) {
			mockLog := ldlogtest.NewMockLog()
			mockLog.Loggers.SetMinLevel(ldlog.Debug)
			store := NewInMemoryDataStore(mockLog.Loggers)
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
	store := makeInMemoryStore()
	require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

	result, err := store.GetAll(datakinds.Features)
	require.NoError(t, err)
	assert.Len(t, result, 0)

	flag1 := ldbuilders.NewFlagBuilder("flag1").Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").Build()
	segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()
	override1 := ldbuilders.NewConfigOverrideBuilder("override1").Build()
	metric1 := ldbuilders.NewMetricBuilder("metric1").Build()
	_, err = store.Upsert(datakinds.Features, flag1.Key, sharedtest.FlagDescriptor(flag1))
	require.NoError(t, err)
	_, err = store.Upsert(datakinds.Features, flag2.Key, sharedtest.FlagDescriptor(flag2))
	require.NoError(t, err)
	_, err = store.Upsert(datakinds.Segments, segment1.Key, sharedtest.SegmentDescriptor(segment1))
	require.NoError(t, err)
	_, err = store.Upsert(datakinds.ConfigOverrides, override1.Key, sharedtest.ConfigOverrideDescriptor(override1))
	require.NoError(t, err)
	_, err = store.Upsert(datakinds.Metrics, metric1.Key, sharedtest.MetricDescriptor(metric1))
	require.NoError(t, err)

	flags, err := store.GetAll(datakinds.Features)
	require.NoError(t, err)
	segments, err := store.GetAll(datakinds.Segments)
	require.NoError(t, err)
	overrides, err := store.GetAll(datakinds.ConfigOverrides)
	require.NoError(t, err)
	metrics, err := store.GetAll(datakinds.Metrics)
	require.NoError(t, err)

	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	expected := extractCollections(sharedtest.NewDataSetBuilder().Flags(flag1, flag2).Segments(segment1).ConfigOverrides(override1).Metrics(metric1).Build())
	assert.Equal(t, expected, [][]ldstoretypes.KeyedItemDescriptor{flags, segments, overrides, metrics})

	result, err = store.GetAll(unknownDataKind{})
	require.NoError(t, err)
	assert.Len(t, result, 0)
}

func testInMemoryDataStoreUpsert(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem dataItemCreator) {
		t.Run("newer version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			updated, err := store.Upsert(kind, "key", item1)
			require.NoError(t, err)
			assert.True(t, updated)

			item1a := makeItem("key", item1.Version+1, true)
			updated, err = store.Upsert(kind, "key", item1a)
			require.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1a, result)
		})

		t.Run("older version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			updated, err := store.Upsert(kind, "key", item1)
			require.NoError(t, err)
			assert.True(t, updated)

			item1a := makeItem("key", item1.Version-1, true)
			updated, err = store.Upsert(kind, "key", item1a)
			require.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})

		t.Run("same version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			updated, err := store.Upsert(kind, "key", item1)
			require.NoError(t, err)
			assert.True(t, updated)

			item1a := makeItem("key", item1.Version, true)
			updated, err = store.Upsert(kind, "key", item1a)
			require.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})
}

func testInMemoryDataStoreDelete(t *testing.T) {
	forAllDataKinds(t, func(t *testing.T, kind ldstoretypes.DataKind, makeItem dataItemCreator) {
		t.Run("newer version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			updated, err := store.Upsert(kind, "key", item1)
			require.NoError(t, err)
			assert.True(t, updated)

			item1a := ldstoretypes.ItemDescriptor{Version: item1.Version + 1, Item: nil}
			updated, err = store.Upsert(kind, "key", item1a)
			require.NoError(t, err)
			assert.True(t, updated)

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1a, result)
		})

		t.Run("older version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			updated, err := store.Upsert(kind, "key", item1)
			require.NoError(t, err)
			assert.True(t, updated)

			item1a := ldstoretypes.ItemDescriptor{Version: item1.Version - 1, Item: nil}
			updated, err = store.Upsert(kind, "key", item1a)
			require.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})

		t.Run("same version", func(t *testing.T) {
			store := makeInMemoryStore()
			require.NoError(t, store.Init(sharedtest.NewDataSetBuilder().Build()))

			item1 := makeItem("key", 10, false)
			updated, err := store.Upsert(kind, "key", item1)
			require.NoError(t, err)
			assert.True(t, updated)

			item1a := ldstoretypes.ItemDescriptor{Version: item1.Version, Item: nil}
			updated, err = store.Upsert(kind, "key", item1a)
			require.NoError(t, err)
			assert.False(t, updated)

			result, err := store.Get(kind, "key")
			require.NoError(t, err)
			assert.Equal(t, item1, result)
		})
	})
}

package datasource

import (
	"strings"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func TestComputeDependenciesFromFlag(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key").Build()
	assert.Len(
		t,
		computeDependenciesFrom(intf.DataKindFeatures(), sharedtest.FlagDescriptor(flag1)),
		0,
	)

	flag2 := ldbuilders.NewFlagBuilder("key").
		AddPrerequisite("flag2", 0).
		AddPrerequisite("flag3", 0).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String("ignore")),
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String("segment1"), ldvalue.String("segment2")),
			),
		).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String("segment3")),
			),
		).
		Build()
	assert.Equal(
		t,
		kindAndKeySet{
			{intf.DataKindFeatures(), "flag2"}:    true,
			{intf.DataKindFeatures(), "flag3"}:    true,
			{intf.DataKindSegments(), "segment1"}: true,
			{intf.DataKindSegments(), "segment2"}: true,
			{intf.DataKindSegments(), "segment3"}: true,
		},
		computeDependenciesFrom(intf.DataKindFeatures(), sharedtest.FlagDescriptor(flag2)),
	)

	flag3 := ldbuilders.NewFlagBuilder("key").
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String("ignore")),
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String("segment1"), ldvalue.String("segment2")),
			),
		).
		Build()
	assert.Equal(
		t,
		kindAndKeySet{
			{intf.DataKindSegments(), "segment1"}: true,
			{intf.DataKindSegments(), "segment2"}: true,
		},
		computeDependenciesFrom(intf.DataKindFeatures(), sharedtest.FlagDescriptor(flag3)),
	)
}

func TestComputeDependenciesFromSegment(t *testing.T) {
	segment := ldbuilders.NewSegmentBuilder("segment").Build()
	assert.Len(
		t,
		computeDependenciesFrom(intf.DataKindSegments(), intf.StoreItemDescriptor{Version: segment.Version, Item: &segment}),
		0,
	)
}

func TestComputeDependenciesFromUnknownDataKind(t *testing.T) {
	assert.Len(
		t,
		computeDependenciesFrom(sharedtest.MockData, intf.StoreItemDescriptor{Version: 1, Item: "x"}),
		0,
	)
}

func TestComputeDependenciesFromNullItem(t *testing.T) {
	assert.Len(
		t,
		computeDependenciesFrom(intf.DataKindFeatures(), intf.StoreItemDescriptor{Version: 1, Item: nil}),
		0,
	)
}

func TestSortCollectionsForDataStoreInit(t *testing.T) {
	inputData := makeDependencyOrderingDataSourceTestData()
	sortedData := sortCollectionsForDataStoreInit(inputData)
	verifySortedData(t, sortedData, inputData)
}

func TestSortCollectionsLeavesItemsOfUnknownDataKindUnchanged(t *testing.T) {
	item1 := sharedtest.MockDataItem{Key: "item1"}
	item2 := sharedtest.MockDataItem{Key: "item2"}
	flag := ldbuilders.NewFlagBuilder("a").Build()
	inputData := []intf.StoreCollection{
		{sharedtest.MockData, []intf.StoreKeyedItemDescriptor{
			{item1.Key, item1.ToItemDescriptor()},
			{item2.Key, item2.ToItemDescriptor()},
		}},
		{intf.DataKindFeatures(), []intf.StoreKeyedItemDescriptor{
			{"a", sharedtest.FlagDescriptor(flag)},
		}},
		{intf.DataKindSegments(), nil},
	}
	sortedData := sortCollectionsForDataStoreInit(inputData)

	// the unknown data kind appears last, and the ordering of its items is unchanged
	assert.Len(t, sortedData, 3)
	assert.Equal(t, intf.DataKindSegments(), sortedData[0].Kind)
	assert.Equal(t, intf.DataKindFeatures(), sortedData[1].Kind)
	assert.Equal(t, sharedtest.MockData, sortedData[2].Kind)
	assert.Equal(t, inputData[0].Items, sortedData[2].Items)
}

func TestDependencyTrackerReturnsSingleValueResultForUnknownItem(t *testing.T) {
	dt := newDependencyTracker()

	// a change to any item with no known depenencies affects only itself
	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag1", kindAndKey{intf.DataKindFeatures(), "flag1"})
}

func TestDependencyTrackerBuildsGraph(t *testing.T) {
	dt := newDependencyTracker()

	flag1 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag2", 0).
		AddPrerequisite("flag3", 0).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String("segment1"), ldvalue.String("segment2")),
			),
		).
		Build()
	dt.updateDependenciesFrom(intf.DataKindFeatures(), flag1.Key, intf.StoreItemDescriptor{Version: flag1.Version, Item: &flag1})

	flag2 := ldbuilders.NewFlagBuilder("flag2").
		AddPrerequisite("flag4", 0).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String("segment2")),
			),
		).
		Build()
	dt.updateDependenciesFrom(intf.DataKindFeatures(), flag2.Key, intf.StoreItemDescriptor{Version: flag2.Version, Item: &flag2})

	// a change to flag1 affects only flag1
	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag1",
		kindAndKey{intf.DataKindFeatures(), "flag1"},
	)

	// a change to flag2 affects flag2 and flag1
	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag2",
		kindAndKey{intf.DataKindFeatures(), "flag2"},
		kindAndKey{intf.DataKindFeatures(), "flag1"},
	)

	// a change to flag3 affects flag3 and flag1
	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag3",
		kindAndKey{intf.DataKindFeatures(), "flag3"},
		kindAndKey{intf.DataKindFeatures(), "flag1"},
	)

	// a change to segment1 affects segment1 and flag1
	verifyDependencyAffectedItems(t, dt, intf.DataKindSegments(), "segment1",
		kindAndKey{intf.DataKindSegments(), "segment1"},
		kindAndKey{intf.DataKindFeatures(), "flag1"},
	)

	// a change to segment2 affects segment2, flag1, and flag2
	verifyDependencyAffectedItems(t, dt, intf.DataKindSegments(), "segment2",
		kindAndKey{intf.DataKindSegments(), "segment2"},
		kindAndKey{intf.DataKindFeatures(), "flag1"},
		kindAndKey{intf.DataKindFeatures(), "flag2"},
	)
}

func TestDependencyTrackerUpdatesGraph(t *testing.T) {
	dt := newDependencyTracker()

	flag1 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag3", 0).
		Build()
	dt.updateDependenciesFrom(intf.DataKindFeatures(), flag1.Key, intf.StoreItemDescriptor{Version: flag1.Version, Item: &flag1})

	flag2 := ldbuilders.NewFlagBuilder("flag2").
		AddPrerequisite("flag3", 0).
		Build()
	dt.updateDependenciesFrom(intf.DataKindFeatures(), flag2.Key, intf.StoreItemDescriptor{Version: flag2.Version, Item: &flag2})

	// at this point, a change to flag3 affects flag3, flag2, and flag1
	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag3",
		kindAndKey{intf.DataKindFeatures(), "flag3"},
		kindAndKey{intf.DataKindFeatures(), "flag2"},
		kindAndKey{intf.DataKindFeatures(), "flag1"},
	)

	// now make it so flag1 now depends on flag4 instead of flag2
	flag1v2 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag4", 0).
		Build()
	dt.updateDependenciesFrom(intf.DataKindFeatures(), flag1.Key, intf.StoreItemDescriptor{Version: flag1v2.Version, Item: &flag1v2})

	// now, a change to flag3 affects flag3 and flag2
	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag3",
		kindAndKey{intf.DataKindFeatures(), "flag3"},
		kindAndKey{intf.DataKindFeatures(), "flag2"},
	)

	// and a change to flag4 affects flag4 and flag1
	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag4",
		kindAndKey{intf.DataKindFeatures(), "flag4"},
		kindAndKey{intf.DataKindFeatures(), "flag1"},
	)
}

func TestDependencyTrackerResetsGraph(t *testing.T) {
	dt := newDependencyTracker()

	flag1 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag3", 0).
		Build()
	dt.updateDependenciesFrom(intf.DataKindFeatures(), flag1.Key, intf.StoreItemDescriptor{Version: flag1.Version, Item: &flag1})

	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag3",
		kindAndKey{intf.DataKindFeatures(), "flag3"},
		kindAndKey{intf.DataKindFeatures(), "flag1"},
	)

	dt.reset()

	verifyDependencyAffectedItems(t, dt, intf.DataKindFeatures(), "flag3",
		kindAndKey{intf.DataKindFeatures(), "flag3"},
	)
}

func verifyDependencyAffectedItems(
	t *testing.T,
	dt *dependencyTracker,
	kind intf.StoreDataKind,
	key string,
	expected ...kindAndKey,
) {
	expectedSet := make(kindAndKeySet)
	for _, value := range expected {
		expectedSet.add(value)
	}
	result := make(kindAndKeySet)
	dt.addAffectedItems(result, kindAndKey{kind, key})
	assert.Equal(t, expectedSet, result)
}

func makeDependencyOrderingDataSourceTestData() []intf.StoreCollection {
	return sharedtest.NewDataSetBuilder().
		Flags(
			ldbuilders.NewFlagBuilder("a").AddPrerequisite("b", 0).AddPrerequisite("c", 0).Build(),
			ldbuilders.NewFlagBuilder("b").AddPrerequisite("c", 0).AddPrerequisite("e", 0).Build(),
			ldbuilders.NewFlagBuilder("c").Build(),
			ldbuilders.NewFlagBuilder("d").Build(),
			ldbuilders.NewFlagBuilder("e").Build(),
			ldbuilders.NewFlagBuilder("f").Build(),
		).
		Segments(
			ldbuilders.NewSegmentBuilder("1").Build(),
		).
		Build()
}

func verifySortedData(t *testing.T, sortedData []intf.StoreCollection, inputData []intf.StoreCollection) {
	assert.Len(t, sortedData, len(inputData))

	assert.Equal(t, intf.DataKindSegments(), sortedData[0].Kind) // Segments should always be first
	assert.Equal(t, intf.DataKindFeatures(), sortedData[1].Kind)

	inputDataMap := fullDataSetToMap(inputData)
	assert.Len(t, sortedData[0].Items, len(inputDataMap[intf.DataKindSegments()]))
	assert.Len(t, sortedData[1].Items, len(inputDataMap[intf.DataKindFeatures()]))

	flags := sortedData[1].Items
	findFlagIndex := func(key string) int {
		for i, item := range flags {
			if item.Key == key {
				return i
			}
		}
		return -1
	}

	for _, item := range inputData[0].Items {
		if flag, ok := item.Item.Item.(*ldmodel.FeatureFlag); ok {
			flagIndex := findFlagIndex(item.Key)
			for _, prereq := range flag.Prerequisites {
				prereqIndex := findFlagIndex(prereq.Key)
				if prereqIndex > flagIndex {
					keys := make([]string, 0, len(flags))
					for _, item := range flags {
						keys = append(keys, item.Key)
					}
					assert.True(t, false, "%s depends on %s, but %s was listed first; keys in order are [%s]",
						flag.Key, prereq.Key, strings.Join(keys, ", "))
				}
			}
		}
	}
}

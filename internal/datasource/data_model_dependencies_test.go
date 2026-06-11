package datasource

import (
	"strings"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/toposort"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
)

func TestComputeDependenciesFromFlag(t *testing.T) {
	flag1 := ldbuilders.NewFlagBuilder("key").Build()
	assert.Len(
		t,
		toposort.GetNeighbors(datakinds.Features, sharedtest.FlagDescriptor(flag1)),
		0,
	)

	flag2 := ldbuilders.NewFlagBuilder("key").
		AddPrerequisite("flag2", 0).
		AddPrerequisite("flag3", 0).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.Clause("key", ldmodel.OperatorIn, ldvalue.String("ignore")),
				ldbuilders.SegmentMatchClause("segment1", "segment2"),
			),
		).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.SegmentMatchClause("segment3"),
			),
		).
		Build()
	assert.Equal(
		t,
		toposort.Neighbors{
			toposort.NewVertex(datakinds.Features, "flag2"):    struct{}{},
			toposort.NewVertex(datakinds.Features, "flag3"):    struct{}{},
			toposort.NewVertex(datakinds.Segments, "segment1"): struct{}{},
			toposort.NewVertex(datakinds.Segments, "segment2"): struct{}{},
			toposort.NewVertex(datakinds.Segments, "segment3"): struct{}{},
		},
		toposort.GetNeighbors(datakinds.Features, sharedtest.FlagDescriptor(flag2)),
	)

	flag3 := ldbuilders.NewFlagBuilder("key").
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.Clause("key}", ldmodel.OperatorIn, ldvalue.String("ignore")),
				ldbuilders.SegmentMatchClause("segment1", "segment2"),
			),
		).
		Build()
	assert.Equal(
		t,
		toposort.Neighbors{
			toposort.NewVertex(datakinds.Segments, "segment1"): struct{}{},
			toposort.NewVertex(datakinds.Segments, "segment2"): struct{}{},
		},
		toposort.GetNeighbors(datakinds.Features, sharedtest.FlagDescriptor(flag3)),
	)
}

func TestComputeDependenciesFromSegment(t *testing.T) {
	segment := ldbuilders.NewSegmentBuilder("segment").Build()
	assert.Len(
		t,
		toposort.GetNeighbors(datakinds.Segments, st.ItemDescriptor{Version: segment.Version, Item: &segment}),
		0,
	)
}

func TestComputeDependenciesFromSegmentWithSegmentReferences(t *testing.T) {
	segment1 := ldbuilders.NewSegmentBuilder("segment1").
		AddRule(ldbuilders.NewSegmentRuleBuilder().Clauses(
			ldbuilders.SegmentMatchClause("segment2", "segment3"),
		)).
		Build()
	assert.Equal(
		t,
		toposort.Neighbors{
			toposort.NewVertex(datakinds.Segments, "segment2"): struct{}{},
			toposort.NewVertex(datakinds.Segments, "segment3"): struct{}{},
		},
		toposort.GetNeighbors(datakinds.Segments, st.ItemDescriptor{Version: segment1.Version, Item: &segment1}),
	)
}

func TestComputeDependenciesFromUnknownDataKind(t *testing.T) {
	assert.Len(
		t,
		toposort.GetNeighbors(mocks.MockData, st.ItemDescriptor{Version: 1, Item: "x"}),
		0,
	)
}

func TestComputeDependenciesFromNullItem(t *testing.T) {
	assert.Len(
		t,
		toposort.GetNeighbors(datakinds.Features, st.ItemDescriptor{Version: 1, Item: nil}),
		0,
	)
}

func TestSortCollectionsForDataStoreInit(t *testing.T) {
	inputData := makeDependencyOrderingDataSourceTestData()
	sortedData := toposort.Sort(inputData)
	verifySortedData(t, sortedData, inputData)
}

func TestSortCollectionsLeavesItemsOfUnknownDataKindUnchanged(t *testing.T) {
	item1 := mocks.MockDataItem{Key: "item1"}
	item2 := mocks.MockDataItem{Key: "item2"}
	flag := ldbuilders.NewFlagBuilder("a").Build()
	inputData := []st.Collection{
		{Kind: mocks.MockData,
			Items: []st.KeyedItemDescriptor{
				{Key: item1.Key, Item: item1.ToItemDescriptor()},
				{Key: item2.Key, Item: item2.ToItemDescriptor()},
			}},
		{Kind: datakinds.Features,
			Items: []st.KeyedItemDescriptor{
				{Key: "a", Item: sharedtest.FlagDescriptor(flag)},
			}},
		{Kind: datakinds.Segments, Items: nil},
	}
	sortedData := toposort.Sort(inputData)

	// the unknown data kind appears last, and the ordering of its items is unchanged
	assert.Len(t, sortedData, 3)
	assert.Equal(t, datakinds.Segments, sortedData[0].Kind)
	assert.Equal(t, datakinds.Features, sortedData[1].Kind)
	assert.Equal(t, mocks.MockData, sortedData[2].Kind)
	assert.Equal(t, inputData[0].Items, sortedData[2].Items)
}

func TestDependencyTrackerReturnsSingleValueResultForUnknownItem(t *testing.T) {
	dt := newDependencyTracker()

	// a change to any item with no known depenencies affects only itself
	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag1", toposort.NewVertex(datakinds.Features, "flag1"))
}

func TestDependencyTrackerBuildsGraph(t *testing.T) {
	dt := newDependencyTracker()

	segment3 := ldbuilders.NewSegmentBuilder("segment3").Build()
	segment2 := ldbuilders.NewSegmentBuilder("segment2").
		AddRule(ldbuilders.NewSegmentRuleBuilder().Clauses(
			ldbuilders.SegmentMatchClause(segment3.Key),
		)).
		Build()
	segment1 := ldbuilders.NewSegmentBuilder("segment1").Build()

	flag1 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag2", 0).
		AddPrerequisite("flag3", 0).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.SegmentMatchClause(segment1.Key, segment2.Key),
			),
		).
		Build()

	flag2 := ldbuilders.NewFlagBuilder("flag2").
		AddPrerequisite("flag4", 0).
		AddRule(
			ldbuilders.NewRuleBuilder().Clauses(
				ldbuilders.SegmentMatchClause(segment2.Key),
			),
		).
		Build()

	for _, s := range []ldmodel.Segment{segment1, segment2, segment3} {
		dt.updateDependenciesFrom(datakinds.Segments, s.Key, sharedtest.SegmentDescriptor(s))
	}
	for _, f := range []ldmodel.FeatureFlag{flag1, flag2} {
		dt.updateDependenciesFrom(datakinds.Features, f.Key, sharedtest.FlagDescriptor(f))
	}

	// a change to flag1 affects only flag1
	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag1",
		toposort.NewVertex(datakinds.Features, "flag1"),
	)

	// a change to flag2 affects flag2 and flag1
	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag2",
		toposort.NewVertex(datakinds.Features, "flag2"),
		toposort.NewVertex(datakinds.Features, "flag1"),
	)

	// a change to flag3 affects flag3 and flag1
	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag3",
		toposort.NewVertex(datakinds.Features, "flag3"),
		toposort.NewVertex(datakinds.Features, "flag1"),
	)

	// a change to segment1 affects segment1 and flag1
	verifyDependencyAffectedItems(t, dt, datakinds.Segments, "segment1",
		toposort.NewVertex(datakinds.Segments, "segment1"),
		toposort.NewVertex(datakinds.Features, "flag1"),
	)

	// a change to segment2 affects segment2, flag1, and flag2
	verifyDependencyAffectedItems(t, dt, datakinds.Segments, "segment2",
		toposort.NewVertex(datakinds.Segments, "segment2"),
		toposort.NewVertex(datakinds.Features, "flag1"),
		toposort.NewVertex(datakinds.Features, "flag2"),
	)

	// a change to segment3 affects segment2, which affects flag1 and flag2
	verifyDependencyAffectedItems(t, dt, datakinds.Segments, "segment3",
		toposort.NewVertex(datakinds.Segments, "segment3"),
		toposort.NewVertex(datakinds.Segments, "segment2"),
		toposort.NewVertex(datakinds.Features, "flag1"),
		toposort.NewVertex(datakinds.Features, "flag2"),
	)
}

func TestDependencyTrackerUpdatesGraph(t *testing.T) {
	dt := newDependencyTracker()

	flag1 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag3", 0).
		Build()
	dt.updateDependenciesFrom(datakinds.Features, flag1.Key, st.ItemDescriptor{Version: flag1.Version, Item: &flag1})

	flag2 := ldbuilders.NewFlagBuilder("flag2").
		AddPrerequisite("flag3", 0).
		Build()
	dt.updateDependenciesFrom(datakinds.Features, flag2.Key, st.ItemDescriptor{Version: flag2.Version, Item: &flag2})

	// at this point, a change to flag3 affects flag3, flag2, and flag1
	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag3",
		toposort.NewVertex(datakinds.Features, "flag3"),
		toposort.NewVertex(datakinds.Features, "flag2"),
		toposort.NewVertex(datakinds.Features, "flag1"),
	)

	// now make it so flag1 now depends on flag4 instead of flag2
	flag1v2 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag4", 0).
		Build()
	dt.updateDependenciesFrom(datakinds.Features, flag1.Key, st.ItemDescriptor{Version: flag1v2.Version, Item: &flag1v2})

	// now, a change to flag3 affects flag3 and flag2
	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag3",
		toposort.NewVertex(datakinds.Features, "flag3"),
		toposort.NewVertex(datakinds.Features, "flag2"),
	)

	// and a change to flag4 affects flag4 and flag1
	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag4",
		toposort.NewVertex(datakinds.Features, "flag4"),
		toposort.NewVertex(datakinds.Features, "flag1"),
	)
}

func TestDependencyTrackerResetsGraph(t *testing.T) {
	dt := newDependencyTracker()

	flag1 := ldbuilders.NewFlagBuilder("flag1").
		AddPrerequisite("flag3", 0).
		Build()
	dt.updateDependenciesFrom(datakinds.Features, flag1.Key, st.ItemDescriptor{Version: flag1.Version, Item: &flag1})

	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag3",
		toposort.NewVertex(datakinds.Features, "flag3"),
		toposort.NewVertex(datakinds.Features, "flag1"),
	)

	dt.reset()

	verifyDependencyAffectedItems(t, dt, datakinds.Features, "flag3",
		toposort.NewVertex(datakinds.Features, "flag3"),
	)
}

func verifyDependencyAffectedItems(
	t *testing.T,
	dt *dependencyTracker,
	kind st.DataKind,
	key string,
	expected ...toposort.Vertex,
) {
	expectedSet := make(toposort.Neighbors)
	for _, value := range expected {
		expectedSet.Add(value)
	}
	result := make(toposort.Neighbors)
	dt.addAffectedItems(result, toposort.NewVertex(kind, key))
	assert.Equal(t, expectedSet, result)
}

func makeDependencyOrderingDataSourceTestData() []st.Collection {
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

func verifySortedData(t *testing.T, sortedData []st.Collection, inputData []st.Collection) {
	assert.Len(t, sortedData, len(inputData))

	assert.Equal(t, datakinds.Segments, sortedData[0].Kind) // Segments should always be first
	assert.Equal(t, datakinds.Features, sortedData[1].Kind)

	inputDataMap := fullDataSetToMap(inputData)
	assert.Len(t, sortedData[0].Items, len(inputDataMap[datakinds.Segments]))
	assert.Len(t, sortedData[1].Items, len(inputDataMap[datakinds.Features]))

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

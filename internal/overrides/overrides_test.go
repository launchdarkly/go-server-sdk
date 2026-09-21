package overrides

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBaseStore is a minimal ReadOnlyStore for testing the overlay and sink against
// arbitrary base data and initialization states.
type fakeBaseStore struct {
	flags       map[string]st.ItemDescriptor
	segments    map[string]st.ItemDescriptor
	initialized bool
	getAllErr   error
}

func (f *fakeBaseStore) items(kind st.DataKind) map[string]st.ItemDescriptor {
	switch kind {
	case datakinds.Features:
		return f.flags
	case datakinds.Segments:
		return f.segments
	}
	return nil
}

func (f *fakeBaseStore) Get(kind st.DataKind, key string) (st.ItemDescriptor, error) {
	if item, ok := f.items(kind)[key]; ok {
		return item, nil
	}
	return st.ItemDescriptor{}.NotFound(), nil
}

func (f *fakeBaseStore) GetAll(kind st.DataKind) ([]st.KeyedItemDescriptor, error) {
	if f.getAllErr != nil {
		return nil, f.getAllErr
	}
	var result []st.KeyedItemDescriptor
	for key, item := range f.items(kind) {
		result = append(result, st.KeyedItemDescriptor{Key: key, Item: item})
	}
	return result, nil
}

func (f *fakeBaseStore) IsInitialized() bool { return f.initialized }

func flagCollection(flags ...ldmodel.FeatureFlag) st.Collection {
	coll := st.Collection{Kind: datakinds.Features}
	for _, flag := range flags {
		coll.Items = append(coll.Items,
			st.KeyedItemDescriptor{Key: flag.Key, Item: sharedtest.FlagDescriptor(flag)})
	}
	return coll
}

func segmentCollection(segments ...ldmodel.Segment) st.Collection {
	coll := st.Collection{Kind: datakinds.Segments}
	for _, segment := range segments {
		coll.Items = append(coll.Items,
			st.KeyedItemDescriptor{Key: segment.Key, Item: sharedtest.SegmentDescriptor(segment)})
	}
	return coll
}

func requireFlag(t *testing.T, item st.ItemDescriptor) *ldmodel.FeatureFlag {
	t.Helper()
	flag, ok := item.Item.(*ldmodel.FeatureFlag)
	require.True(t, ok, "expected a flag item")
	return flag
}

func TestLayerMarksCopiesWithoutMutatingSource(t *testing.T) {
	layer := NewLayer()
	flag := ldbuilders.NewFlagBuilder("flag1").Version(2).Build()
	segment := ldbuilders.NewSegmentBuilder("segment1").Version(3).Build()

	layer.SetAll([]st.Collection{flagCollection(flag), segmentCollection(segment)})

	assert.False(t, flag.IsOverride, "source flag must not be mutated")
	assert.False(t, segment.IsOverride, "source segment must not be mutated")

	storedFlag, ok := layer.Get(datakinds.Features, "flag1")
	require.True(t, ok)
	assert.True(t, requireFlag(t, storedFlag).IsOverride)
	assert.Equal(t, 2, storedFlag.Version)

	storedSegment, ok := layer.Get(datakinds.Segments, "segment1")
	require.True(t, ok)
	assert.True(t, storedSegment.Item.(*ldmodel.Segment).IsOverride)
}

// rawOverrideFlag builds a flag by struct literal, so it carries no preprocessing caches. The
// builders and the deserializer would add them. Without caches, any write by the layer into
// the entity's rules, clauses, or targets is visible as a new cache.
func rawOverrideFlag() ldmodel.FeatureFlag {
	return ldmodel.FeatureFlag{
		Key:        "flag1",
		Version:    1,
		On:         true,
		Variations: []ldvalue.Value{ldvalue.Bool(false), ldvalue.Bool(true)},
		Targets:    []ldmodel.Target{{Variation: 1, Values: []string{"user-a", "user-b"}}},
		Rules: []ldmodel.FlagRule{{
			ID: "rule",
			Clauses: []ldmodel.Clause{{
				Attribute: ldattr.NewLiteralRef("name"),
				Op:        ldmodel.OperatorIn,
				Values:    []ldvalue.Value{ldvalue.String("x"), ldvalue.String("y")},
			}},
			VariationOrRollout: ldmodel.VariationOrRollout{Variation: ldvalue.NewOptionalInt(1)},
		}},
		Fallthrough: ldmodel.VariationOrRollout{Variation: ldvalue.NewOptionalInt(0)},
	}
}

// rawOverrideSegment is the segment counterpart of rawOverrideFlag.
func rawOverrideSegment() ldmodel.Segment {
	return ldmodel.Segment{
		Key:              "segment1",
		Version:          1,
		IncludedContexts: []ldmodel.SegmentTarget{{ContextKind: "user", Values: []string{"user-a"}}},
		ExcludedContexts: []ldmodel.SegmentTarget{{ContextKind: "user", Values: []string{"user-z"}}},
		Rules: []ldmodel.SegmentRule{{
			ID: "rule",
			Clauses: []ldmodel.Clause{{
				Attribute: ldattr.NewLiteralRef("name"),
				Op:        ldmodel.OperatorIn,
				Values:    []ldvalue.Value{ldvalue.String("x"), ldvalue.String("y")},
			}},
		}},
	}
}

// A source may retain the entities it supplies. The layer must not write into them, not even
// into the unexported cache fields of their nested rules, clauses, and targets.
func TestLayerDoesNotWriteIntoRetainedEntities(t *testing.T) {
	// Step 1: the source builds its entities and keeps them. A pristine twin of each is built
	// the same way and is never handed to the layer.
	suppliedFlag, pristineFlag := rawOverrideFlag(), rawOverrideFlag()
	suppliedSegment, pristineSegment := rawOverrideSegment(), rawOverrideSegment()
	require.True(t, reflect.DeepEqual(suppliedFlag, pristineFlag))
	require.True(t, reflect.DeepEqual(suppliedSegment, pristineSegment))

	// Step 2: the source supplies the entities. The layer stores marked copies.
	layer := NewLayer()
	layer.SetAll([]st.Collection{flagCollection(suppliedFlag), segmentCollection(suppliedSegment)})

	// Step 3: the supplied entities still equal their twins. The deep comparison covers every
	// field, including the unexported caches inside the shared rules, clauses, and targets.
	assert.True(t, reflect.DeepEqual(suppliedFlag, pristineFlag), "the layer wrote into the supplied flag")
	assert.True(t, reflect.DeepEqual(suppliedSegment, pristineSegment), "the layer wrote into the supplied segment")

	// Step 4: the stored copies are marked.
	storedFlagItem, ok := layer.Get(datakinds.Features, "flag1")
	require.True(t, ok)
	storedFlag := requireFlag(t, storedFlagItem)
	assert.True(t, storedFlag.IsOverride)
	storedSegmentItem, ok := layer.Get(datakinds.Segments, "segment1")
	require.True(t, ok)
	storedSegment, ok := storedSegmentItem.Item.(*ldmodel.Segment)
	require.True(t, ok)
	assert.True(t, storedSegment.IsOverride)

	// Step 5: the stored copies evaluate without caches. The accessors scan the values.
	accessors := ldmodel.EvaluatorAccessors
	assert.True(t, accessors.ClauseFindValue(&storedFlag.Rules[0].Clauses[0], ldvalue.String("y")))
	assert.False(t, accessors.ClauseFindValue(&storedFlag.Rules[0].Clauses[0], ldvalue.String("z")))
	assert.True(t, accessors.TargetFindKey(&storedFlag.Targets[0], "user-b"))
	assert.False(t, accessors.TargetFindKey(&storedFlag.Targets[0], "user-c"))
	assert.True(t, accessors.SegmentTargetFindKey(&storedSegment.IncludedContexts[0], "user-a"))
	assert.False(t, accessors.SegmentTargetFindKey(&storedSegment.IncludedContexts[0], "user-z"))
	assert.True(t, accessors.SegmentTargetFindKey(&storedSegment.ExcludedContexts[0], "user-z"))
	assert.True(t, accessors.ClauseFindValue(&storedSegment.Rules[0].Clauses[0], ldvalue.String("x")))
}

// layerHasFlag reports whether the layer holds a flag entry for the key.
func layerHasFlag(layer *Layer, key string) bool {
	_, ok := layer.Get(datakinds.Features, key)
	return ok
}

func TestLayerReplacementSemantics(t *testing.T) {
	layer := NewLayer()
	assert.True(t, layer.IsEmpty())
	assert.False(t, layerHasFlag(layer, "flag1"))

	layer.SetAll([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag1").Build())})
	assert.False(t, layer.IsEmpty())
	assert.True(t, layerHasFlag(layer, "flag1"))

	// A replacement is a full snapshot: entries absent from it are removed.
	layer.SetAll([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag2").Build())})
	assert.False(t, layerHasFlag(layer, "flag1"))
	assert.True(t, layerHasFlag(layer, "flag2"))

	layer.SetAll(nil)
	assert.True(t, layer.IsEmpty())
	assert.False(t, layerHasFlag(layer, "flag2"))
}

func TestOverlayGetPrecedence(t *testing.T) {
	base := &fakeBaseStore{
		flags: map[string]st.ItemDescriptor{
			"both":      sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("both").Version(1).Build()),
			"base-only": sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("base-only").Version(1).Build()),
		},
		initialized: true,
	}
	layer := NewLayer()
	layer.SetAll([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("both").Version(99).Build(),
		ldbuilders.NewFlagBuilder("override-only").Version(1).Build(),
	)})
	overlay := NewOverlay(base, layer)

	item, err := overlay.Get(datakinds.Features, "both")
	require.NoError(t, err)
	assert.Equal(t, 99, item.Version)
	assert.True(t, requireFlag(t, item).IsOverride)

	item, err = overlay.Get(datakinds.Features, "base-only")
	require.NoError(t, err)
	assert.False(t, requireFlag(t, item).IsOverride)

	item, err = overlay.Get(datakinds.Features, "override-only")
	require.NoError(t, err)
	assert.True(t, requireFlag(t, item).IsOverride)

	item, err = overlay.Get(datakinds.Features, "nowhere")
	require.NoError(t, err)
	assert.Nil(t, item.Item)
}

func TestOverlayGetServesOverridesFromUninitializedBase(t *testing.T) {
	base := &fakeBaseStore{initialized: false}
	layer := NewLayer()
	layer.SetAll([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag1").Build())})
	overlay := NewOverlay(base, layer)

	item, err := overlay.Get(datakinds.Features, "flag1")
	require.NoError(t, err)
	assert.True(t, requireFlag(t, item).IsOverride)
	assert.False(t, overlay.IsInitialized())
}

func TestOverlayGetAllUnion(t *testing.T) {
	base := &fakeBaseStore{
		flags: map[string]st.ItemDescriptor{
			"both":      sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("both").Version(1).Build()),
			"base-only": sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("base-only").Version(1).Build()),
			"tombstone": {Version: 5, Item: nil},
		},
		initialized: true,
	}
	layer := NewLayer()
	layer.SetAll([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("both").Version(99).Build(),
		ldbuilders.NewFlagBuilder("tombstone").Version(1).Build(),
		ldbuilders.NewFlagBuilder("override-only").Version(1).Build(),
	)})
	overlay := NewOverlay(base, layer)

	items, err := overlay.GetAll(datakinds.Features)
	require.NoError(t, err)
	byKey := map[string]st.ItemDescriptor{}
	for _, item := range items {
		byKey[item.Key] = item.Item
	}
	require.Len(t, byKey, 4)
	assert.Equal(t, 99, byKey["both"].Version)
	assert.True(t, requireFlag(t, byKey["both"]).IsOverride)
	assert.False(t, requireFlag(t, byKey["base-only"]).IsOverride)
	assert.NotNil(t, byKey["tombstone"].Item, "override must win over a deleted-item tombstone")
	assert.True(t, requireFlag(t, byKey["override-only"]).IsOverride)
}

func TestOverlayGetAllWithEmptyLayerIsPassthrough(t *testing.T) {
	base := &fakeBaseStore{
		flags:       map[string]st.ItemDescriptor{"flag1": sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("flag1").Build())},
		initialized: true,
	}
	overlay := NewOverlay(base, NewLayer())
	items, err := overlay.GetAll(datakinds.Features)
	require.NoError(t, err)
	require.Len(t, items, 1)

	base.getAllErr = errors.New("sinkhole")
	_, err = overlay.GetAll(datakinds.Features)
	assert.Error(t, err)
}

func TestOverlayGetAllServesOverridesWhenBaseFails(t *testing.T) {
	base := &fakeBaseStore{getAllErr: errors.New("sinkhole"), initialized: true}
	layer := NewLayer()
	layer.SetAll([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("override-1").Version(1).Build(),
		ldbuilders.NewFlagBuilder("override-2").Version(2).Build(),
	)})
	overlay := NewOverlay(base, layer)

	items, err := overlay.GetAll(datakinds.Features)
	require.NoError(t, err)
	require.Len(t, items, 2)
	keys := map[string]bool{}
	for _, item := range items {
		keys[item.Key] = true
		assert.True(t, requireFlag(t, item.Item).IsOverride)
	}
	assert.True(t, keys["override-1"])
	assert.True(t, keys["override-2"])
}

type sinkFixture struct {
	base     *fakeBaseStore
	layer    *Layer
	sink     *Sink
	notified []string
	listen   bool
}

func newSinkFixture(base *fakeBaseStore) *sinkFixture {
	f := &sinkFixture{base: base, layer: NewLayer(), listen: true}
	f.sink = NewSink(f.layer, base,
		func(key string) { f.notified = append(f.notified, key) },
		func() bool { return f.listen },
		ldlog.NewDisabledLoggers())
	return f
}

func (f *sinkFixture) takeNotified() []string {
	result := f.notified
	f.notified = nil
	sort.Strings(result)
	return result
}

func TestSinkNotifiesOnAddChangeRemove(t *testing.T) {
	base := &fakeBaseStore{
		flags: map[string]st.ItemDescriptor{
			"flag1": sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("flag1").Version(1).Build()),
		},
		initialized: true,
	}
	f := newSinkFixture(base)

	// Adding an override is a change even though flag1 also exists in base data.
	f.sink.SetOverrides([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
		ldbuilders.NewFlagBuilder("flag2").Version(1).Build(),
	)})
	assert.Equal(t, []string{"flag1", "flag2"}, f.takeNotified())

	// An identical replacement (rebuilt from scratch, new pointers) changes nothing.
	f.sink.SetOverrides([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
		ldbuilders.NewFlagBuilder("flag2").Version(1).Build(),
	)})
	assert.Empty(t, f.takeNotified())

	// Changing one entry notifies only that entry.
	f.sink.SetOverrides([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
		ldbuilders.NewFlagBuilder("flag2").Version(2).Build(),
	)})
	assert.Equal(t, []string{"flag2"}, f.takeNotified())

	// Removing overrides notifies them: flag1 reverts to base data, flag2 to not-found.
	f.sink.SetOverrides(nil)
	assert.Equal(t, []string{"flag1", "flag2"}, f.takeNotified())
}

func TestSinkSegmentOverrideFansOutToDependentFlags(t *testing.T) {
	flagWithSegment := ldbuilders.NewFlagBuilder("dependent").Version(1).
		AddRule(ldbuilders.NewRuleBuilder().ID("r").Variation(0).
			Clauses(ldbuilders.SegmentMatchClause("segment1"))).
		Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		Build()
	base := &fakeBaseStore{
		flags: map[string]st.ItemDescriptor{
			"dependent": sharedtest.FlagDescriptor(flagWithSegment),
			"unrelated": sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("unrelated").Version(1).Build()),
		},
		segments: map[string]st.ItemDescriptor{
			"segment1": sharedtest.SegmentDescriptor(ldbuilders.NewSegmentBuilder("segment1").Version(1).Build()),
		},
		initialized: true,
	}
	f := newSinkFixture(base)

	f.sink.SetOverrides([]st.Collection{segmentCollection(
		ldbuilders.NewSegmentBuilder("segment1").Version(99).Build(),
	)})
	// The segment itself is not a flag, so only the dependent flag is notified.
	assert.Equal(t, []string{"dependent"}, f.takeNotified())
}

func TestSinkPrerequisiteFanOutUsesOldAndNewViews(t *testing.T) {
	// The override for "parent" declares a prerequisite on "prereq". The base definition of
	// "parent" has no prerequisites. When the override is removed, the dependency edge only
	// exists in the old merged view. "parent" must still be notified when "prereq" changes
	// in the same replacement.
	parentOverride := ldbuilders.NewFlagBuilder("parent").Version(1).
		AddPrerequisite("prereq", 0).Build()
	base := &fakeBaseStore{
		flags: map[string]st.ItemDescriptor{
			"parent": sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("parent").Version(1).Build()),
			"prereq": sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("prereq").Version(1).Build()),
		},
		initialized: true,
	}
	f := newSinkFixture(base)

	f.sink.SetOverrides([]st.Collection{flagCollection(parentOverride)})
	assert.Equal(t, []string{"parent"}, f.takeNotified())

	// Replace the layer with an override of the prerequisite only. The "parent" override is
	// removed (a change) and "prereq" is added (a change). Fan-out through the old view's
	// edge also reaches "parent".
	f.sink.SetOverrides([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("prereq").Version(99).Build(),
	)})
	assert.Equal(t, []string{"parent", "prereq"}, f.takeNotified())

	// Now only "prereq" is overridden and nothing depends on it in the new view either.
	f.sink.SetOverrides([]st.Collection{flagCollection(
		ldbuilders.NewFlagBuilder("prereq").Version(100).Build(),
	)})
	assert.Equal(t, []string{"prereq"}, f.takeNotified())
}

func TestSinkSkipsDiffWorkWithoutListeners(t *testing.T) {
	base := &fakeBaseStore{getAllErr: errors.New("GetAll should not be called"), initialized: true}
	f := newSinkFixture(base)
	f.listen = false

	f.sink.SetOverrides([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag1").Build())})
	assert.True(t, layerHasFlag(f.layer, "flag1"))
	assert.Empty(t, f.notified)
}

func TestSinkToleratesBaseReadFailure(t *testing.T) {
	base := &fakeBaseStore{getAllErr: errors.New("sinkhole"), initialized: true}
	f := newSinkFixture(base)

	// Fan-out degrades, but the directly changed flags are still notified.
	f.sink.SetOverrides([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag1").Build())})
	assert.Equal(t, []string{"flag1"}, f.takeNotified())
}

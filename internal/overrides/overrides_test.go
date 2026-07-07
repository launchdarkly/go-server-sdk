package overrides

import (
	"errors"
	"sort"
	"testing"

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

func TestLayerReplacementSemantics(t *testing.T) {
	layer := NewLayer()
	assert.True(t, layer.IsEmpty())
	assert.False(t, layer.HasFlag("flag1"))

	layer.SetAll([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag1").Build())})
	assert.False(t, layer.IsEmpty())
	assert.True(t, layer.HasFlag("flag1"))

	// A replacement is a full snapshot: entries absent from it are removed.
	layer.SetAll([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag2").Build())})
	assert.False(t, layer.HasFlag("flag1"))
	assert.True(t, layer.HasFlag("flag2"))

	layer.SetAll(nil)
	assert.True(t, layer.IsEmpty())
	assert.False(t, layer.HasFlag("flag2"))
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
	// The override for "parent" declares a prerequisite on "prereq"; the base definition of
	// "parent" has no prerequisites. When the override is removed, the dependency edge only
	// exists in the old merged view, and "parent" must still be notified when "prereq"
	// changes in the same replacement.
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

	// Replace the layer with an override of the prerequisite only: "parent"'s override is
	// removed (a change) and "prereq" is added (a change); fan-out through the old view's
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
	assert.True(t, f.layer.HasFlag("flag1"))
	assert.Empty(t, f.notified)
}

func TestSinkToleratesBaseReadFailure(t *testing.T) {
	base := &fakeBaseStore{getAllErr: errors.New("sinkhole"), initialized: true}
	f := newSinkFixture(base)

	// Fan-out degrades, but the directly changed flags are still notified.
	f.sink.SetOverrides([]st.Collection{flagCollection(ldbuilders.NewFlagBuilder("flag1").Build())})
	assert.Equal(t, []string{"flag1"}, f.takeNotified())
}

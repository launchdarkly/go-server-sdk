package subsystems

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeParsedFlagItem(key string, version int) ldstoretypes.ItemDescriptor {
	flag := ldbuilders.NewFlagBuilder(key).Version(version).SingleVariation(ldvalue.Bool(true)).Build()
	return ldstoretypes.ItemDescriptor{Version: version, Item: &flag}
}

func serializeFlagItem(item ldstoretypes.ItemDescriptor) []byte {
	return datakinds.Features.Serialize(item)
}

func TestChangeSetBuilderAddParsedPutPrePopulatesCollections(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})

	// The raw object bytes are deliberately NOT valid JSON. If Collections() were to re-parse
	// them, it would fail; succeeding proves the pre-parsed items were used instead.
	builder.AddParsedPut(FlagKind, "flag-1", 1, []byte("deliberately not JSON"), makeParsedFlagItem("flag-1", 1))
	builder.AddDelete(FlagKind, "flag-2", 2)

	changeSet, err := builder.Finish(NewSelector("state", 1))
	require.NoError(t, err)

	// The raw bytes are preserved verbatim for consumers of Changes().
	changes := changeSet.Changes()
	require.Len(t, changes, 2)
	assert.Equal(t, "deliberately not JSON", string(changes[0].Object))

	collections, err := changeSet.Collections()
	require.NoError(t, err)
	require.Len(t, collections, 1)
	assert.Equal(t, "features", collections[0].Kind.GetName())
	require.Len(t, collections[0].Items, 2)

	byKey := map[string]ldstoretypes.ItemDescriptor{}
	for _, item := range collections[0].Items {
		byKey[item.Key] = item.Item
	}
	require.IsType(t, &ldmodel.FeatureFlag{}, byKey["flag-1"].Item)
	assert.Equal(t, 1, byKey["flag-1"].Version)
	assert.Nil(t, byKey["flag-2"].Item) // the delete is a tombstone
	assert.Equal(t, 2, byKey["flag-2"].Version)
}

func TestChangeSetBuilderMixedPutsFallBackToLazyParsing(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})

	item1 := makeParsedFlagItem("flag-1", 1)
	builder.AddParsedPut(FlagKind, "flag-1", 1, serializeFlagItem(item1), item1)
	// This put has no parsed item, so the whole change set must be parsed lazily.
	builder.AddPut(FlagKind, "flag-2", 2, serializeFlagItem(makeParsedFlagItem("flag-2", 2)))

	changeSet, err := builder.Finish(NewSelector("state", 1))
	require.NoError(t, err)

	collections, err := changeSet.Collections()
	require.NoError(t, err)
	require.Len(t, collections, 1)
	assert.Len(t, collections[0].Items, 2)
	for _, item := range collections[0].Items {
		assert.NotNil(t, item.Item.Item, "item %s should have been parsed", item.Key)
	}
}

func TestChangeSetBuilderUnparsedPutOfUnknownKindDoesNotBlockPrePopulation(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})

	// An unrecognized kind never has a parsed item, and is excluded from collections; its
	// presence must not force the lazy path (which the invalid raw bytes of the parsed put
	// would then fail on).
	builder.AddPut("future-kind", "future-key", 1, []byte(`{"anything":true}`))
	builder.AddParsedPut(FlagKind, "flag-1", 1, []byte("deliberately not JSON"), makeParsedFlagItem("flag-1", 1))

	changeSet, err := builder.Finish(NewSelector("state", 1))
	require.NoError(t, err)

	// Both changes are visible to Changes() consumers...
	require.Len(t, changeSet.Changes(), 2)
	assert.Equal(t, ObjectKind("future-kind"), changeSet.Changes()[0].Kind)

	// ...but only the recognized kind appears in collections.
	collections, err := changeSet.Collections()
	require.NoError(t, err)
	require.Len(t, collections, 1)
	assert.Equal(t, "features", collections[0].Kind.GetName())
	assert.Len(t, collections[0].Items, 1)
}

func TestChangeSetBuilderParsedPutsAcrossFinishReuse(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})
	builder.AddParsedPut(FlagKind, "flag-1", 1, []byte("raw-1"), makeParsedFlagItem("flag-1", 1))
	first, err := builder.Finish(NewSelector("state", 1))
	require.NoError(t, err)

	// The builder is reusable after Finish; the next change set starts empty and the intent
	// becomes xfer-changes.
	builder.AddParsedPut(FlagKind, "flag-2", 2, []byte("raw-2"), makeParsedFlagItem("flag-2", 2))
	second, err := builder.Finish(NewSelector("state", 2))
	require.NoError(t, err)

	require.Len(t, first.Changes(), 1)
	assert.Equal(t, "flag-1", first.Changes()[0].Key)
	assert.Equal(t, IntentTransferFull, first.IntentCode())

	require.Len(t, second.Changes(), 1)
	assert.Equal(t, "flag-2", second.Changes()[0].Key)
	assert.Equal(t, IntentTransferChanges, second.IntentCode())

	firstCollections, err := first.Collections()
	require.NoError(t, err)
	secondCollections, err := second.Collections()
	require.NoError(t, err)
	require.Len(t, firstCollections, 1)
	require.Len(t, secondCollections, 1)
	assert.Equal(t, "flag-1", firstCollections[0].Items[0].Key)
	assert.Equal(t, "flag-2", secondCollections[0].Items[0].Key)
}

package subsystems

import (
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/stretchr/testify/assert"
)

func TestChangeSetBuilder_New(t *testing.T) {
	builder := NewChangeSetBuilder()
	assert.NotNil(t, builder)
}

func TestChangeSetBuilder_MustStartToFinish(t *testing.T) {
	builder := NewChangeSetBuilder()
	selector := NewSelector("foo", 1)
	_, err := builder.Finish(selector)
	assert.Error(t, err)

	assert.NoError(t, builder.Start(ServerIntent{Payload: Payload{Code: IntentNone}}))

	_, err = builder.Finish(selector)
	assert.NoError(t, err)
}

func TestChangeSetBuilder_Changes(t *testing.T) {
	builder := NewChangeSetBuilder()
	err := builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferChanges}})
	assert.NoError(t, err)

	builder.AddPut("foo", "bar", 1, []byte("baz"))
	builder.AddDelete("foo", "bar", 1)

	selector := NewSelector("foo", 1)
	changeSet, err := builder.Finish(selector)
	assert.NoError(t, err)
	assert.NotNil(t, changeSet)

	changes := changeSet.Changes()
	assert.Equal(t, 2, len(changes))
	assert.Equal(t, Change{Action: ChangeTypePut, Kind: "foo", Key: "bar", Version: 1, Object: []byte("baz")}, changes[0])
	assert.Equal(t, Change{Action: ChangeTypeDelete, Kind: "foo", Key: "bar", Version: 1}, changes[1])

	assert.Equal(t, IntentTransferChanges, changeSet.IntentCode())
	assert.Equal(t, selector, changeSet.Selector())

}

// After receiving an intent, the SDK may receive 1 or more objects before receiving a payload-transferred.
// At that point, LaunchDarkly may send more objects followed by another payload-transferred. These objects
// should be regarded as part of an implicit "xfer-changes" intent, even though the server doesn't actually send one.
// If the server intends to use an xfer-full instead (for efficiency or other reasons), it will need to explicitly
// send one.
func TestChangeSetBuilder_ImplicitIntentXferChanges(t *testing.T) {
	builder := NewChangeSetBuilder()
	err := builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})
	assert.NoError(t, err)

	changes1, err := builder.Finish(NewSelector("foo", 1))
	assert.NoError(t, err)
	assert.Equal(t, IntentTransferFull, changes1.IntentCode())

	builder.AddPut("foo", "bar", 1, []byte("baz"))
	changes2, err := builder.Finish(NewSelector("bar", 2))
	assert.NoError(t, err)

	assert.Equal(t, IntentTransferChanges, changes2.IntentCode())
}

func TestChangeSetBuilder_NoChanges(t *testing.T) {
	builder := NewChangeSetBuilder()
	changeSet := builder.NoChanges()
	assert.NotNil(t, changeSet)

	intent := changeSet.IntentCode()
	assert.NotNil(t, intent)

	assert.Equal(t, IntentNone, intent)

	assert.False(t, changeSet.Selector().IsDefined())
	assert.Equal(t, NoSelector(), changeSet.Selector())
}

func TestChangeSet_Collections_EmptyChanges(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})
	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	collections, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections)
	assert.Equal(t, 0, len(collections))
}

func TestChangeSet_Collections_PutChanges(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})

	flagJSON := []byte(`{"key":"flag1","version":1}`)
	segmentJSON := []byte(`{"key":"seg1","version":2}`)

	builder.AddPut(FlagKind, "flag1", 1, flagJSON)
	builder.AddPut(SegmentKind, "seg1", 2, segmentJSON)

	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	collections, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections)
	assert.Equal(t, 2, len(collections))

	// Verify we have both flags and segments collections
	var foundFlags, foundSegments bool
	for _, collection := range collections {
		if len(collection.Items) > 0 {
			if collection.Items[0].Key == "flag1" {
				foundFlags = true
				assert.Equal(t, 1, collection.Items[0].Item.Version)
				assert.NotNil(t, collection.Items[0].Item.Item)
			}
			if collection.Items[0].Key == "seg1" {
				foundSegments = true
				assert.Equal(t, 2, collection.Items[0].Item.Version)
				assert.NotNil(t, collection.Items[0].Item.Item)
			}
		}
	}
	assert.True(t, foundFlags, "Expected to find flags collection")
	assert.True(t, foundSegments, "Expected to find segments collection")
}

func TestChangeSet_Collections_DeleteChanges(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferChanges}})

	builder.AddDelete(FlagKind, "flag1", 5)
	builder.AddDelete(SegmentKind, "seg1", 10)

	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	collections, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections)
	assert.Equal(t, 2, len(collections))

	// Verify tombstones (nil items with versions)
	for _, collection := range collections {
		if len(collection.Items) > 0 {
			item := collection.Items[0]
			assert.Nil(t, item.Item.Item, "Deleted items should have nil Item")
			if item.Key == "flag1" {
				assert.Equal(t, 5, item.Item.Version)
			} else if item.Key == "seg1" {
				assert.Equal(t, 10, item.Item.Version)
			}
		}
	}
}

func TestChangeSet_Collections_MixedChanges(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferChanges}})

	flagJSON := []byte(`{"key":"flag1","version":1}`)

	builder.AddPut(FlagKind, "flag1", 1, flagJSON)
	builder.AddDelete(FlagKind, "flag2", 2)
	builder.AddPut(SegmentKind, "seg1", 3, []byte(`{"key":"seg1","version":3}`))

	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	collections, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections)

	// Find the flags collection
	var flagsCollection *ldstoretypes.Collection
	for i := range collections {
		for _, item := range collections[i].Items {
			if item.Key == "flag1" || item.Key == "flag2" {
				flagsCollection = &collections[i]
				break
			}
		}
	}

	assert.NotNil(t, flagsCollection, "Expected to find flags collection")
	assert.Equal(t, 2, len(flagsCollection.Items), "Expected 2 flag items")

	// Verify the put and delete
	for _, item := range flagsCollection.Items {
		if item.Key == "flag1" {
			assert.NotNil(t, item.Item.Item, "Put item should have non-nil Item")
			assert.Equal(t, 1, item.Item.Version)
		} else if item.Key == "flag2" {
			assert.Nil(t, item.Item.Item, "Deleted item should have nil Item")
			assert.Equal(t, 2, item.Item.Version)
		}
	}
}

func TestChangeSet_Collections_Caching(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})

	flagJSON := []byte(`{"key":"flag1","version":1}`)
	builder.AddPut(FlagKind, "flag1", 1, flagJSON)

	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	// First call
	collections1, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections1)

	// Second call should return the cached result (same pointer)
	collections2, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections2)

	// Verify they are the same slice (cached)
	assert.Equal(t, &collections1[0], &collections2[0], "Expected cached collections to be the same slice")
}

func TestChangeSet_Collections_UnknownKind(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferChanges}})

	// Add an unknown kind that won't convert to FDV1
	builder.AddPut(ObjectKind("unknown"), "item1", 1, []byte(`{"key":"item1"}`))
	// Also add a valid flag
	builder.AddPut(FlagKind, "flag1", 2, []byte(`{"key":"flag1","version":2}`))

	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	collections, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections)

	// Should only have 1 collection (the flag), unknown kind should be ignored
	assert.Equal(t, 1, len(collections))
	assert.Equal(t, 1, len(collections[0].Items))
	assert.Equal(t, "flag1", collections[0].Items[0].Key)
}

func TestChangeSet_Collections_InvalidJSON(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferChanges}})

	// Add invalid JSON for a put
	builder.AddPut(FlagKind, "flag1", 1, []byte(`{invalid json`))

	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	collections, err := changeSet.Collections()
	assert.Error(t, err, "Expected error for invalid JSON")
	assert.Nil(t, collections)
}

func TestChangeSet_Collections_MultipleItemsSameKind(t *testing.T) {
	builder := NewChangeSetBuilder()
	builder.Start(ServerIntent{Payload: Payload{Code: IntentTransferFull}})

	// Add multiple flags
	builder.AddPut(FlagKind, "flag1", 1, []byte(`{"key":"flag1","version":1}`))
	builder.AddPut(FlagKind, "flag2", 2, []byte(`{"key":"flag2","version":2}`))
	builder.AddPut(FlagKind, "flag3", 3, []byte(`{"key":"flag3","version":3}`))

	changeSet, err := builder.Finish(NewSelector("test", 1))
	assert.NoError(t, err)

	collections, err := changeSet.Collections()
	assert.NoError(t, err)
	assert.NotNil(t, collections)

	// Should have 1 collection with 3 items
	assert.Equal(t, 1, len(collections))
	assert.Equal(t, 3, len(collections[0].Items))

	// Verify all flags are present
	keys := make(map[string]bool)
	for _, item := range collections[0].Items {
		keys[item.Key] = true
	}
	assert.True(t, keys["flag1"])
	assert.True(t, keys["flag2"])
	assert.True(t, keys["flag3"])
}

package fdv2proto

import (
	"testing"

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

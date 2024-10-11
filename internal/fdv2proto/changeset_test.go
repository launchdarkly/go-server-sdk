package fdv2proto

import (
	"github.com/stretchr/testify/assert"
	"testing"
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

	assert.NoError(t, builder.Start(ServerIntent{Payloads: []Payload{{Code: IntentNone}}}))

	_, err = builder.Finish(selector)
	assert.NoError(t, err)
}

func TestChangeSetBuilder_MustHaveAtLeastOnePayload(t *testing.T) {
	builder := NewChangeSetBuilder()
	err := builder.Start(ServerIntent{})
	assert.Error(t, err)

	err = builder.Start(ServerIntent{Payloads: []Payload{{Code: IntentNone}}})
	assert.NoError(t, err)

	err = builder.Start(ServerIntent{Payloads: []Payload{{Code: IntentNone}, {Code: IntentNone}, {Code: IntentNone}}})
	assert.NoError(t, err)
}

func TestChangeSetBuilder_Changes(t *testing.T) {
	builder := NewChangeSetBuilder()
	err := builder.Start(ServerIntent{Payloads: []Payload{{Code: IntentTransferChanges}}})
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

	assert.Equal(t, IntentTransferChanges, changeSet.Intent().Payloads[0].Code)
	assert.Equal(t, selector, changeSet.Selector())

}

func TestChangeSetBuilder_ImplicitXferChanges(t *testing.T) {

}

func TestChangeSetBuilder_NoChanges(t *testing.T) {
	builder := NewChangeSetBuilder()
	changeSet := builder.NoChanges()
	assert.NotNil(t, changeSet)

	intent := changeSet.Intent()
	assert.NotNil(t, intent)

	assert.NotEmpty(t, intent.Payloads)
	assert.Equal(t, IntentNone, intent.Payloads[0].Code)

	assert.False(t, changeSet.Selector().IsDefined())
	assert.Equal(t, NoSelector(), changeSet.Selector())
}

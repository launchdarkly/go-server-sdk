package datasystem

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

func noopBuilder() (subsystems.DataSynchronizer, error) {
	return nil, nil
}

func TestSynchronizerListNextSkipsBlockedSlotsAndWraps(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder, noopBuilder}, nil)

	// Select the first slot, then block the second slot.
	list.next()
	assert.Equal(t, 0, list.currentIndex())
	list.next()
	list.blockCurrent()

	// Moving on from the blocked slot reaches the third slot, then wraps past the blocked slot.
	list.next()
	assert.Equal(t, 2, list.currentIndex())
	list.next()
	assert.Equal(t, 0, list.currentIndex())

	// The blocked slot stays in the list but is not counted as available.
	assert.Len(t, list.slots, 3)
	assert.Equal(t, 2, list.availableCount())
}

func TestSynchronizerListNextKeepsOnlyAvailableSlot(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	list.next()
	list.blockCurrent()

	// Moving on from the blocked slot selects the only available slot.
	list.next()
	// Moving on again keeps the same slot.
	list.next()
	assert.Equal(t, 1, list.currentIndex())
}

func TestSynchronizerListNextLeavesNoCurrentSlotWhenAllSlotsBlocked(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	list.next()
	list.blockCurrent()
	list.next()
	list.blockCurrent()

	// No slot remains available.
	list.next()
	assert.Equal(t, -1, list.currentIndex())
	assert.Equal(t, 0, list.availableCount())
}

func TestSynchronizerListEmptyHasNoAvailableSlot(t *testing.T) {
	list := newSynchronizerList(nil, nil)

	// An empty list has nothing to select.
	list.next()
	assert.Equal(t, -1, list.currentIndex())
	assert.Equal(t, 0, list.availableCount())

	// Building with no current slot returns an error.
	_, err := list.build()
	assert.Error(t, err)
}

func TestSynchronizerListResetReturnsToFirstAvailableSlot(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder, noopBuilder}, nil)
	list.next()
	list.blockCurrent()
	list.next()
	list.next()
	assert.Equal(t, 2, list.currentIndex())
	assert.False(t, list.isFirstAvailable())

	// Reset and select again.
	list.reset()
	list.next()

	// The first slot is blocked, so the second slot is the first available slot.
	assert.Equal(t, 1, list.currentIndex())
	assert.True(t, list.isFirstAvailable())
}

func TestSynchronizerListFDv1FallbackSlotIsBlockedUntilFallback(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, noopBuilder)
	assert.True(t, list.hasFDv1Fallback())

	// Before a fallback, selection never reaches the FDv1 slot.
	assert.Equal(t, 2, list.availableCount())
	list.next()
	list.next()
	list.next()
	assert.Equal(t, 0, list.currentIndex())

	// Fall back to FDv1 and select again.
	list.fdv1Fallback()
	list.next()

	// Only the FDv1 slot is available, and every FDv2 slot is blocked.
	assert.Equal(t, 2, list.currentIndex())
	assert.True(t, list.slots[list.currentIndex()].isFDv1Fallback)
	assert.Equal(t, 1, list.availableCount())
}

func TestSynchronizerListBlockAllLeavesNoSlotAvailable(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, noopBuilder)
	list.next()

	// Block every slot, including the FDv1 fallback slot.
	list.blockAll()

	// No slot remains available.
	list.next()
	assert.Equal(t, -1, list.currentIndex())
}

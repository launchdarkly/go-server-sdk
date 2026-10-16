package datasystem

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

func noopBuilder() (subsystems.DataSynchronizer, error) {
	return nil, nil
}

func TestSynchronizerListParkedSlotIsIneligibleUntilUnparkTime(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	now := time.Now()
	list.next()

	// While the unpark time is in the future, only the other slot is eligible.
	list.slots[0].parkedUntil = now.Add(5 * time.Minute)
	assert.Equal(t, 1, list.eligibleCount())

	// Once it has passed, the slot is eligible again.
	list.slots[0].parkedUntil = now.Add(-time.Second)
	assert.Equal(t, 2, list.eligibleCount())
}

func TestSynchronizerListParkDurationDoublesToTheCeiling(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder}, nil)
	list.next()

	for _, want := range []time.Duration{
		5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 40 * time.Minute, time.Hour, time.Hour,
	} {
		delay := list.parkCurrent()
		assert.Greater(t, delay, want/2)
		assert.LessOrEqual(t, delay, want)
	}
}

func TestSynchronizerListResetReturnsParksToTheInitialDelay(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder}, nil)
	list.next()

	first := list.parkCurrent()
	list.parkCurrent()
	list.resetCurrentBackoff()
	afterReset := list.parkCurrent()

	for _, delay := range []time.Duration{first, afterReset} {
		assert.Greater(t, delay, 150*time.Second)
		assert.LessOrEqual(t, delay, 5*time.Minute)
	}
}

func TestSynchronizerListNextAfterParkPrefersTheSlotAfterTheParkedOne(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder, noopBuilder}, nil)
	now := time.Now()

	// Fall back from the first slot to the second without parking the first.
	list.next()
	list.next()
	assert.Equal(t, 1, list.currentIndex())

	// Park the second slot. Selection continues after it rather than returning to the first, which
	// a fallback condition demoted but did not park.
	list.slots[1].parkedUntil = now.Add(5 * time.Minute)
	list.next()
	assert.Equal(t, 2, list.currentIndex())
}

func TestSynchronizerListNextWrapsToMostPreferredEligibleSlot(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	now := time.Now()

	// Move to the last slot, then park it. The search wraps to the first slot.
	list.next()
	list.next()
	assert.Equal(t, 1, list.currentIndex())
	list.slots[1].parkedUntil = now.Add(5 * time.Minute)
	list.next()
	assert.Equal(t, 0, list.currentIndex())
}

func TestSynchronizerListNextKeepsOnlyEligibleSlot(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	now := time.Now()
	list.next()
	list.slots[0].parkedUntil = now.Add(5 * time.Minute)

	// Moving on from the parked slot selects the only eligible slot, and moving on again keeps it.
	// Stopping the only runnable slot would leave the SDK with no source of updates.
	list.next()
	list.next()
	assert.Equal(t, 1, list.currentIndex())
}

func TestSynchronizerListNextLeavesNoCurrentSlotWhenEverySlotIsParked(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	now := time.Now()
	list.slots[0].parkedUntil = now.Add(5 * time.Minute)
	list.slots[1].parkedUntil = now.Add(10 * time.Minute)

	// No slot is eligible, so nothing is selected and nothing starts.
	list.next()
	assert.Equal(t, -1, list.currentIndex())
	assert.Equal(t, 0, list.eligibleCount())
}

func TestSynchronizerListEarliestUnparkReportsTheSoonestParkedSlot(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	now := time.Now()

	// With nothing parked there is no unpark time to wait for.
	_, ok := list.earliestUnpark()
	assert.False(t, ok)

	// Park both slots. The earliest of the two unpark times is reported.
	list.slots[0].parkedUntil = now.Add(10 * time.Minute)
	list.slots[1].parkedUntil = now.Add(5 * time.Minute)

	unpark, ok := list.earliestUnpark()
	assert.True(t, ok)
	assert.Equal(t, now.Add(5*time.Minute), unpark)
}

func TestSynchronizerListEarliestUnparkIgnoresSlotsPastTheirUnparkTime(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, nil)
	now := time.Now()
	list.next()
	list.slots[0].parkedUntil = now.Add(-time.Second)

	// A slot past its unpark time is eligible, so nothing is waiting.
	_, ok := list.earliestUnpark()
	assert.False(t, ok)
}

func TestSynchronizerListEmptyHasNoEligibleSlot(t *testing.T) {
	list := newSynchronizerList(nil, nil)

	// An empty list has nothing to select.
	list.next()
	assert.Equal(t, -1, list.currentIndex())
	assert.Equal(t, 0, list.eligibleCount())

	// Building with no current slot returns an error.
	_, err := list.build()
	assert.Error(t, err)
}

func TestSynchronizerListResetReturnsToMostPreferredEligibleSlot(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder, noopBuilder}, nil)
	now := time.Now()
	list.next()
	list.slots[0].parkedUntil = now.Add(5 * time.Minute)
	list.next()
	list.next()
	assert.Equal(t, 2, list.currentIndex())
	assert.False(t, list.isFirstEligible())

	// Recovery returns to the most-preferred eligible slot, which is the second while the first is parked.
	list.reset()
	list.next()
	assert.Equal(t, 1, list.currentIndex())
	assert.True(t, list.isFirstEligible())

	// When the first slot re-enters, it becomes the most-preferred eligible slot.
	list.slots[0].parkedUntil = now.Add(-time.Second)
	assert.False(t, list.isFirstEligible())
	list.reset()
	list.next()
	assert.Equal(t, 0, list.currentIndex())
}

func TestSynchronizerListParkingSurvivesSelectionChanges(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder, noopBuilder}, nil)
	now := time.Now()
	list.next()
	list.next()

	// Park the second slot, then move through the rest of the list and back.
	list.slots[1].parkedUntil = now.Add(5 * time.Minute)
	list.next()
	list.next()
	assert.Equal(t, 0, list.currentIndex())

	// The park is still in force; selection never lands on the parked slot.
	list.next()
	assert.Equal(t, 2, list.currentIndex())
}

func TestSynchronizerListFDv1FallbackSlotIsExcludedUntilFallback(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, noopBuilder)
	assert.True(t, list.hasFDv1Fallback())

	// Before a fallback, selection never reaches the FDv1 slot.
	assert.Equal(t, 2, list.eligibleCount())
	list.next()
	list.next()
	list.next()
	assert.Equal(t, 0, list.currentIndex())

	// Fall back to FDv1 and select again.
	list.fallBackToFDv1()
	list.next()

	// Only the FDv1 slot is eligible, and every FDv2 slot is excluded.
	assert.Equal(t, 2, list.currentIndex())
	assert.True(t, list.slots[list.currentIndex()].isFDv1Fallback)
	assert.Equal(t, 1, list.eligibleCount())
}

func TestSynchronizerListExcludedSlotIsNotEligibleWhenItsParkExpires(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, noopBuilder)
	now := time.Now()

	// Park the first slot past its unpark time, then engage a directed fallback to FDv1.
	list.next()
	list.slots[0].parkedUntil = now.Add(-time.Second)
	list.fallBackToFDv1()

	// The FDv2 slot is past its unpark time but excluded, so it still does not run.
	assert.Equal(t, 1, list.eligibleCount())
	list.reset()
	list.next()
	assert.True(t, list.slots[list.currentIndex()].isFDv1Fallback)
}

func TestSynchronizerListExcludeAllLeavesNoSlotEligible(t *testing.T) {
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, noopBuilder)
	list.next()

	// Exclude every slot, including the FDv1 fallback slot.
	list.excludeAll()

	// No slot remains eligible.
	list.next()
	assert.Equal(t, -1, list.currentIndex())
	assert.Equal(t, 0, list.eligibleCount())
}

func TestSynchronizerListUnparkAllKeepsCurrentFDv2Slot(t *testing.T) {
	// Streaming, polling, and FDv1 fallback slots. Streaming is parked, and polling is current.
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, noopBuilder)
	list.next()
	list.parkCurrent()
	list.next()
	assert.Equal(t, 1, list.currentIndex())

	list.unparkAll()

	// Polling stays current, and streaming is eligible again as the first slot.
	assert.Equal(t, 1, list.currentIndex())
	assert.Equal(t, 2, list.eligibleCount())
	assert.False(t, list.isFirstEligible())

	// A recovery selects streaming.
	list.reset()
	list.next()
	assert.Equal(t, 0, list.currentIndex())
}

func TestSynchronizerListUnparkAllStaysOnFDv1AfterFallback(t *testing.T) {
	// Streaming, polling, and FDv1 fallback slots. Streaming is parked, then the list falls back to FDv1.
	list := newSynchronizerList([]synchronizerBuilder{noopBuilder, noopBuilder}, noopBuilder)
	list.next()
	list.parkCurrent()
	list.next()
	list.fallBackToFDv1()
	list.next()
	assert.Equal(t, 2, list.currentIndex())

	list.unparkAll()

	// The FDv1 slot stays current and is the only eligible slot.
	assert.Equal(t, 2, list.currentIndex())
	assert.Equal(t, 1, list.eligibleCount())
}

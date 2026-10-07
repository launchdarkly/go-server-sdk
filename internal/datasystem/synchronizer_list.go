package datasystem

import (
	"errors"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type synchronizerBuilder = func() (subsystems.DataSynchronizer, error)

var errNoCurrentSynchronizer = errors.New("no current synchronizer")

// synchronizerSlot holds one synchronizer builder and its state.
type synchronizerSlot struct {
	build          synchronizerBuilder
	isFDv1Fallback bool

	// True if the synchronizer failed permanently. A blocked slot is not eligible for use.
	blocked bool
}

// synchronizerList is the ordered list of synchronizers that the data system can use. A
// synchronizer that fails permanently stays in the list and is marked as blocked. The FDv1
// fallback synchronizer, if configured, is the last slot.
//
// The list is either in FDv2 mode or in FDv1 mode. In FDv2 mode, only the FDv2 slots are
// eligible. In FDv1 mode, only the FDv1 fallback slot is eligible. The mode and the blocked
// state are independent. A slot is available if it is eligible in the current mode and it is
// not blocked.
//
// The list is not safe for concurrent use. Only the goroutine that runs the synchronizers
// uses it.
type synchronizerList struct {
	slots []synchronizerSlot

	// True if the last slot is the FDv1 fallback slot.
	hasFDv1 bool

	// True after an FDv1 fallback.
	onFDv1 bool

	// Index of the current slot, or -1 if there is no current slot.
	current int
}

func newSynchronizerList(builders []synchronizerBuilder, fdv1Fallback synchronizerBuilder) *synchronizerList {
	slots := make([]synchronizerSlot, 0, len(builders)+1)
	for _, b := range builders {
		slots = append(slots, synchronizerSlot{build: b})
	}
	if fdv1Fallback != nil {
		slots = append(slots, synchronizerSlot{build: fdv1Fallback, isFDv1Fallback: true})
	}
	return &synchronizerList{slots: slots, hasFDv1: fdv1Fallback != nil, current: -1}
}

// isAvailable returns true if the slot is eligible in the current mode and is not blocked.
func (l *synchronizerList) isAvailable(slot synchronizerSlot) bool {
	return !slot.blocked && slot.isFDv1Fallback == l.onFDv1
}

// next makes the next available slot after the current slot the current slot. The search
// wraps around to the start of the list. If there is no current slot, the search starts at
// the start of the list. If the current slot is the only available slot, it stays current.
// If no slot is available, there is no current slot.
func (l *synchronizerList) next() {
	for i := 1; i <= len(l.slots); i++ {
		idx := (l.current + i) % len(l.slots)
		if l.isAvailable(l.slots[idx]) {
			l.current = idx
			return
		}
	}
	l.current = -1
}

// reset clears the current slot. The next call to next selects the first available slot.
func (l *synchronizerList) reset() {
	l.current = -1
}

// currentIndex returns the index of the current slot, or -1 if there is no current slot.
func (l *synchronizerList) currentIndex() int {
	return l.current
}

// build builds a synchronizer from the current slot. It returns an error if there is no
// current slot.
func (l *synchronizerList) build() (subsystems.DataSynchronizer, error) {
	if l.current < 0 {
		return nil, errNoCurrentSynchronizer
	}
	return l.slots[l.current].build()
}

// blockCurrent marks the current slot as blocked.
func (l *synchronizerList) blockCurrent() {
	if l.current >= 0 {
		l.slots[l.current].blocked = true
	}
}

// isFirstAvailable returns true if the current slot is the first available slot.
func (l *synchronizerList) isFirstAvailable() bool {
	for i, slot := range l.slots {
		if l.isAvailable(slot) {
			return i == l.current
		}
	}
	return false
}

// availableCount returns the number of available slots.
func (l *synchronizerList) availableCount() int {
	count := 0
	for _, slot := range l.slots {
		if l.isAvailable(slot) {
			count++
		}
	}
	return count
}

// hasFDv1Fallback returns true if the list contains an FDv1 fallback slot.
func (l *synchronizerList) hasFDv1Fallback() bool {
	return l.hasFDv1
}

// blockAll blocks every slot and clears the current slot.
func (l *synchronizerList) blockAll() {
	for i := range l.slots {
		l.slots[i].blocked = true
	}
	l.current = -1
}

// fdv1Fallback puts the list in FDv1 mode, so only the FDv1 fallback slot is eligible. It
// also clears the current slot, so the next call to next selects the FDv1 fallback slot. The
// blocked state of the FDv2 slots does not change.
func (l *synchronizerList) fdv1Fallback() {
	l.onFDv1 = true
	l.current = -1
}

// unblock clears the blocked state of every slot, including FDv2 slots in FDv1 mode. The
// mode does not change, so after an FDv1 fallback only the FDv1 fallback slot is available.
// The current slot does not change.
func (l *synchronizerList) unblock() {
	for i := range l.slots {
		l.slots[i].blocked = false
	}
}

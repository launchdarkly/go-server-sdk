package datasystem

import (
	"errors"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type synchronizerBuilder = func() (subsystems.DataSynchronizer, error)

var errNoCurrentSynchronizer = errors.New("no current synchronizer")

// synchronizerSlot holds one synchronizer builder and its state. A blocked slot is not
// eligible for use. A slot that is paused for an FDv1 fallback is also blocked, and is
// unblocked again when FDv2 recovers.
type synchronizerSlot struct {
	build          synchronizerBuilder
	isFDv1Fallback bool
	blocked        bool
	pausedForFDv1  bool
}

// synchronizerList is the ordered list of synchronizers that the data system can use. A
// synchronizer that fails permanently stays in the list and is marked as blocked. The FDv1
// fallback synchronizer, if configured, is the last slot. It is blocked until an FDv1
// fallback occurs, and blocked again when FDv2 recovers.
//
// The list is not safe for concurrent use. Only the goroutine that runs the synchronizers
// uses it.
type synchronizerList struct {
	slots []synchronizerSlot

	// True if the last slot is the FDv1 fallback slot.
	hasFDv1 bool

	// Index of the current slot, or -1 if there is no current slot.
	current int
}

func newSynchronizerList(builders []synchronizerBuilder, fdv1Fallback synchronizerBuilder) *synchronizerList {
	slots := make([]synchronizerSlot, 0, len(builders)+1)
	for _, b := range builders {
		slots = append(slots, synchronizerSlot{build: b})
	}
	if fdv1Fallback != nil {
		slots = append(slots, synchronizerSlot{build: fdv1Fallback, isFDv1Fallback: true, blocked: true})
	}
	return &synchronizerList{slots: slots, hasFDv1: fdv1Fallback != nil, current: -1}
}

// next makes the next available slot after the current slot the current slot. The search
// wraps around to the start of the list. If there is no current slot, the search starts at
// the start of the list. If the current slot is the only available slot, it stays current.
// If no slot is available, there is no current slot.
func (l *synchronizerList) next() {
	for i := 1; i <= len(l.slots); i++ {
		idx := (l.current + i) % len(l.slots)
		if !l.slots[idx].blocked {
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
		if !slot.blocked {
			return i == l.current
		}
	}
	return false
}

// availableCount returns the number of slots that are not blocked.
func (l *synchronizerList) availableCount() int {
	count := 0
	for _, slot := range l.slots {
		if !slot.blocked {
			count++
		}
	}
	return count
}

// hasFDv1Fallback returns true if the list contains an FDv1 fallback slot.
func (l *synchronizerList) hasFDv1Fallback() bool {
	return l.hasFDv1
}

// fdv1Fallback pauses every available FDv2 slot and unblocks the FDv1 fallback slot, if there
// is one. It also clears the current slot, so the next call to next selects the FDv1 fallback
// slot.
func (l *synchronizerList) fdv1Fallback() {
	for i := range l.slots {
		slot := &l.slots[i]
		if slot.isFDv1Fallback {
			slot.blocked = false
		} else if !slot.blocked {
			slot.blocked = true
			slot.pausedForFDv1 = true
		}
	}
	l.current = -1
}

// fdv2Recovery undoes fdv1Fallback. It unblocks every FDv2 slot that fdv1Fallback paused and
// blocks the FDv1 fallback slot. An FDv2 slot that was blocked because it failed permanently
// stays blocked. It also clears the current slot, so the next call to next selects the first
// available FDv2 slot.
func (l *synchronizerList) fdv2Recovery() {
	for i := range l.slots {
		slot := &l.slots[i]
		if slot.isFDv1Fallback {
			slot.blocked = true
		} else if slot.pausedForFDv1 {
			slot.blocked = false
			slot.pausedForFDv1 = false
		}
	}
	l.current = -1
}

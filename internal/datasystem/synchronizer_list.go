package datasystem

import (
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type synchronizerBuilder = func() (subsystems.DataSynchronizer, error)

// Bounds on the delay before a parked synchronizer may run again.
const (
	parkInitialDelay = 5 * time.Minute
	parkMaxDelay     = 1 * time.Hour
	afterParkDelay   = 1 * time.Second
)

// The default duration of a directed fallback to FDv1, before jitter.
const defaultFDv1FallbackTTL = 1 * time.Hour

var errNoCurrentSynchronizer = errors.New("no current synchronizer")

// synchronizerSlot holds one synchronizer builder and its state.
type synchronizerSlot struct {
	build          synchronizerBuilder
	isFDv1Fallback bool

	// excluded is true when the slot is not in the set the data system selects from: the FDv1
	// fallback slot before a directive, and the FDv2 slots while one is in effect.
	excluded bool

	// parkedUntil is the unpark time of a parked slot. Zero means not parked.
	parkedUntil time.Time

	// parkAttempts is the number of times the slot has been parked since its last reset.
	parkAttempts int
}

// jittered returns d minus a random jitter of up to half of d.
func jittered(d time.Duration) time.Duration {
	if half := int64(d / 2); half > 0 {
		return d - time.Duration(rand.Int63n(half)) //nolint:gosec // weak RNG is acceptable for jitter
	}
	return d
}

// park parks the slot and returns the delay until its unpark time. Each successive park
// doubles the delay, up to parkMaxDelay.
func (s *synchronizerSlot) park() time.Duration {
	s.parkAttempts++
	t := time.Duration(math.Min(
		float64(parkInitialDelay)*math.Pow(2, float64(s.parkAttempts-1)),
		float64(parkMaxDelay),
	))
	delay := jittered(t)
	s.parkedUntil = time.Now().Add(delay)
	return delay
}

// The list is not safe for concurrent use. Only the goroutine that runs the synchronizers
// uses it.
type synchronizerList struct {
	slots []synchronizerSlot

	// True if the last slot is the FDv1 fallback slot.
	hasFDv1 bool

	// Index of the current slot, or -1 if there is no current slot.
	current int

	// The end of the directed fallback to FDv1. Zero means no fallback is in effect.
	fdv1FallbackUntil time.Time
}

func newSynchronizerList(builders []synchronizerBuilder, fdv1Fallback synchronizerBuilder) *synchronizerList {
	slots := make([]synchronizerSlot, 0, len(builders)+1)
	for _, b := range builders {
		slots = append(slots, synchronizerSlot{build: b})
	}
	if fdv1Fallback != nil {
		slots = append(slots, synchronizerSlot{build: fdv1Fallback, isFDv1Fallback: true, excluded: true})
	}
	return &synchronizerList{
		slots:   slots,
		hasFDv1: fdv1Fallback != nil,
		current: -1,
	}
}

// isParked reports whether the slot's unpark time has not yet arrived.
func (l *synchronizerList) isParked(idx int) bool {
	return time.Now().Before(l.slots[idx].parkedUntil)
}

// isEligible reports whether the slot can be selected.
func (l *synchronizerList) isEligible(idx int) bool {
	return !l.slots[idx].excluded && !l.isParked(idx)
}

// next makes the first eligible slot after the current slot the current slot. The search wraps
// around, so it reaches the most-preferred eligible slot when none follows the current one. If
// there is no current slot, the search starts at the start of the list. If the current slot is
// the only eligible slot, it stays current. If no slot is eligible, there is no current slot.
func (l *synchronizerList) next() {
	for i := 1; i <= len(l.slots); i++ {
		idx := (l.current + i) % len(l.slots)
		if l.isEligible(idx) {
			l.current = idx
			return
		}
	}
	l.current = -1
}

// reset clears the current slot. The next call to next selects the most-preferred eligible slot.
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

// parkCurrent parks the current slot and returns the delay until its unpark time.
func (l *synchronizerList) parkCurrent() time.Duration {
	if l.current < 0 {
		return 0
	}
	return l.slots[l.current].park()
}

// unparkAll unparks every slot, including synchronizers that are currently excluded.
func (l *synchronizerList) unparkAll() {
	for i := range l.slots {
		l.slots[i].parkedUntil = time.Time{}
	}
}

// failoverDelay returns the delay to wait before starting another synchronizer after a park.
func (l *synchronizerList) failoverDelay() time.Duration {
	return jittered(afterParkDelay)
}

// resetCurrentBackoff clears the current slot's park escalation.
func (l *synchronizerList) resetCurrentBackoff() {
	if l.current >= 0 {
		l.slots[l.current].parkAttempts = 0
	}
}

// isCurrentFirstEligible returns true if the current slot is the most-preferred eligible slot.
func (l *synchronizerList) isCurrentFirstEligible() bool {
	for i := range l.slots {
		if l.isEligible(i) {
			return i == l.current
		}
	}
	return false
}

// eligibleCount returns the number of slots that can be selected.
func (l *synchronizerList) eligibleCount() int {
	count := 0
	for i := range l.slots {
		if l.isEligible(i) {
			count++
		}
	}
	return count
}

// earliestUnpark returns the soonest of the unpark times of the parked slots and the end of the
// FDv1 fallback, and whether there is one. Excluded slots are skipped.
func (l *synchronizerList) earliestUnpark() (time.Time, bool) {
	earliest := l.fdv1FallbackUntil
	for i := range l.slots {
		if l.slots[i].excluded || !l.isParked(i) {
			continue
		}
		if earliest.IsZero() || l.slots[i].parkedUntil.Before(earliest) {
			earliest = l.slots[i].parkedUntil
		}
	}
	return earliest, !earliest.IsZero()
}

// hasFDv1Fallback returns true if the list contains an FDv1 fallback slot.
func (l *synchronizerList) hasFDv1Fallback() bool {
	return l.hasFDv1
}

// fallBackToFDv1 excludes every FDv2 slot and includes the FDv1 fallback slot until ttl elapses.
// It also clears the current slot, so the next call to next selects the FDv1 fallback slot. Any
// park on the FDv1 fallback slot stays in force.
func (l *synchronizerList) fallBackToFDv1(ttl time.Duration) {
	for i := range l.slots {
		l.slots[i].excluded = !l.slots[i].isFDv1Fallback
	}
	l.current = -1
	l.fdv1FallbackUntil = time.Now().Add(ttl)
}

// maybeEndFDv1Fallback includes every FDv2 slot and excludes the FDv1 fallback slot if the FDv1
// fallback TTL has elapsed. It reports whether it ended the fallback.
func (l *synchronizerList) maybeEndFDv1Fallback() bool {
	if l.fdv1FallbackUntil.IsZero() || time.Now().Before(l.fdv1FallbackUntil) {
		return false
	}
	for i := range l.slots {
		l.slots[i].excluded = l.slots[i].isFDv1Fallback
	}
	l.fdv1FallbackUntil = time.Time{}
	return true
}

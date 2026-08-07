package datasource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// In the normal regime (no failures observed), NextWait returns exactly PollInterval.
func TestPollingStrategy_NormalRegimeReturnsPollInterval(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)
	assert.Equal(t, 30*time.Second, s.NextWait())
}

// A normal-classified failure advances the attempts counter but does not engage
// the extended regime. Wait is still pinned to PollInterval by the wait floor.
func TestPollingStrategy_NormalFailureDoesNotEngageExtended(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)
	s.OnFailure(FailureClassNormal)

	assert.Equal(t, 1, s.attempts)
	assert.Equal(t, 30*time.Second, s.initialDelay)
	assert.Equal(t, 30*time.Second, s.maxDelay)
	assert.Equal(t, 30*time.Second, s.NextWait())
}

// An unexpected-classified failure engages the extended regime: initialDelay
// becomes the configured extended base and maxDelay becomes the RETRY-spec cap.
func TestPollingStrategy_UnexpectedFailureEngagesExtended(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)
	s.OnFailure(FailureClassUnexpected)

	assert.Equal(t, 5*time.Minute, s.initialDelay)
	assert.Equal(t, time.Hour, s.maxDelay)
}

// Polling spec: initialDelay = max(configured, PollInterval). When PollInterval
// is larger than the configured extended base (mobile-background case), the
// extended regime uses PollInterval as its base. Result: no observable
// differentiation between regimes.
func TestPollingStrategy_ExtendedInitialClampedToPollInterval(t *testing.T) {
	s := newPollingStrategy(time.Hour, 5*time.Minute)
	s.OnFailure(FailureClassUnexpected)

	assert.Equal(t, time.Hour, s.initialDelay)
	assert.Equal(t, time.Hour, s.maxDelay)
	// All subsequent waits collapse to PollInterval.
	assert.Equal(t, time.Hour, s.NextWait())
}

// Extended regime doubles per attempt per RETRY §1.4, capped at
// extendedPollMaxDelay (1 hour). Jitter subtracts up to T/2, so each wait falls
// in [T/2, T] before the (in these cases irrelevant) wait floor.
func TestPollingStrategy_ExtendedDoubling(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)

	tests := []struct {
		n                      int
		lowerBound, upperBound time.Duration
	}{
		{1, 2*time.Minute + 30*time.Second, 5 * time.Minute}, // T = 5m
		{2, 5 * time.Minute, 10 * time.Minute},               // T = 10m
		{3, 10 * time.Minute, 20 * time.Minute},              // T = 20m
		{4, 20 * time.Minute, 40 * time.Minute},              // T = 40m
		{5, 30 * time.Minute, time.Hour},                     // T capped at 60m
		{6, 30 * time.Minute, time.Hour},                     // still capped
	}
	for _, tc := range tests {
		s.OnFailure(FailureClassUnexpected)
		w := s.NextWait()
		assert.GreaterOrEqual(t, w, tc.lowerBound, "n=%d", tc.n)
		assert.LessOrEqual(t, w, tc.upperBound, "n=%d", tc.n)
	}
}

// RETRY §1.4.4 polling B1 override: NextWait never returns less than PollInterval,
// even when the exponential math (or jitter) would drive it below.
func TestPollingStrategy_WaitFloorAtPollInterval(t *testing.T) {
	// Extended base is much smaller than PollInterval. Every wait should be
	// clamped up to PollInterval until doubling grows T past it.
	s := newPollingStrategy(30*time.Second, 1*time.Millisecond)
	for n := 1; n <= 10; n++ {
		s.OnFailure(FailureClassUnexpected)
		assert.GreaterOrEqual(t, s.NextWait(), 30*time.Second, "n=%d", n)
	}
}

// The 2-consecutive-success reset gate: first success flips the gate flag but
// does not clear attempts or exit the extended regime.
func TestPollingStrategy_FirstSuccessDoesNotReset(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()

	assert.True(t, s.priorPollWasSuccessful, "reset gate should be armed")
	assert.Equal(t, 1, s.attempts, "one success must not reset attempts")
	assert.Equal(t, 5*time.Minute, s.initialDelay, "one success must not exit extended regime")
}

// Two consecutive successes clear attempts and return the strategy to the
// normal regime (RETRY §1.8 polling binding).
func TestPollingStrategy_TwoConsecutiveSuccessesReset(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()
	s.OnSuccess()

	assert.Equal(t, 0, s.attempts)
	assert.Equal(t, 30*time.Second, s.initialDelay)
	assert.Equal(t, 30*time.Second, s.maxDelay)
	assert.Equal(t, 30*time.Second, s.NextWait())
}

// A failure between two successes clears the reset gate — reset requires
// STRICTLY consecutive successes, so a first-then-fail-then-first pattern does
// not fire the reset.
func TestPollingStrategy_FailureClearsResetGate(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()                   // gate armed
	s.OnFailure(FailureClassNormal) // gate cleared, attempts=2
	s.OnSuccess()                   // gate armed again but does not fire reset

	assert.True(t, s.priorPollWasSuccessful)
	assert.Equal(t, 2, s.attempts, "reset must not have fired")
}

// A single normal failure after a reset does not re-engage extended-regime
// parameters — extended engagement requires an Unexpected classification.
func TestPollingStrategy_NormalFailureAfterResetStaysNormal(t *testing.T) {
	s := newPollingStrategy(30*time.Second, 5*time.Minute)
	// Engage extended, then reset back to normal.
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()
	s.OnSuccess()
	// A subsequent normal failure must not re-engage extended.
	s.OnFailure(FailureClassNormal)

	assert.Equal(t, 1, s.attempts)
	assert.Equal(t, 30*time.Second, s.initialDelay, "normal failure must not engage extended regime")
	assert.Equal(t, 30*time.Second, s.maxDelay)
}

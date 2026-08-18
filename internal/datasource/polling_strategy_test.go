package datasource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// In the normal regime (no failures observed), NextWait returns exactly PollInterval.
func TestPollingStrategy_NormalRegimeReturnsPollInterval(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)
	assert.Equal(t, 30*time.Second, s.NextWait())
}

// A normal-classified failure advances n but does not engage
// the extended regime. Wait is still pinned to PollInterval by the wait floor.
func TestPollingStrategy_NormalFailureDoesNotEngageExtended(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)
	s.OnFailure(FailureClassNormal)

	assert.Equal(t, 1, s.n)
	assert.Equal(t, 30*time.Second, s.initialDelay)
	assert.Equal(t, 30*time.Second, s.maxDelay)
	assert.Equal(t, 30*time.Second, s.NextWait())
}

// An unexpected-classified failure engages the extended regime: initialDelay
// becomes the configured extended base and maxDelay becomes the RETRY-spec cap.
func TestPollingStrategy_UnexpectedFailureEngagesExtended(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)
	s.OnFailure(FailureClassUnexpected)

	assert.Equal(t, defaultExtendedInitialPollDelay, s.initialDelay)
	assert.Equal(t, time.Hour, s.maxDelay)
}

// Polling spec: initialDelay = max(configured, PollInterval). When PollInterval
// is larger than the configured extended base (mobile-background case), the
// extended regime uses PollInterval as its base. Result: no observable
// differentiation between regimes.
func TestPollingStrategy_ExtendedInitialClampedToPollInterval(t *testing.T) {
	s := newPollingStrategy(time.Hour, defaultExtendedInitialPollDelay)
	s.OnFailure(FailureClassUnexpected)

	assert.Equal(t, time.Hour, s.initialDelay)
	assert.Equal(t, time.Hour, s.maxDelay)
	// All subsequent waits collapse to PollInterval.
	assert.Equal(t, time.Hour, s.NextWait())
}

// After prior normal-classified failures, the first unexpected failure MUST
// reset n to 1 so the first extended-regime wait uses the new initialDelay
// (5min) directly, not initialDelay * 2^k where k is the count of prior
// normal failures. Guards against conflating the two roles of a single
// counter -- formula input (this field, n; resets on regime transition per
// RETRY §1.5.3 / streaming Confluence spec) and total-attempts observability
// (a separate concept not tracked by this struct). Without this behavior, a
// sequence of "normal, normal, unexpected" would inflate the first extended
// wait to 20min or more (bounded by extendedPollMaxDelay).
func TestPollingStrategy_UnexpectedAfterNormalFailuresStartsAtInitialDelay(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)

	// Prior normal failures accumulate. Normal-regime NextWait is bounded by
	// normalInterval regardless of n, so these aren't observable in delay --
	// but they DO advance n.
	s.OnFailure(FailureClassNormal)
	s.OnFailure(FailureClassNormal)
	s.OnFailure(FailureClassNormal)
	assert.Equal(t, 3, s.n, "n should advance on normal failures")

	// First unexpected failure. Transition into extended regime must reset
	// n to 1 so the first extended-regime formula yields
	// initialDelay * 2^0 = initialDelay = 5min.
	s.OnFailure(FailureClassUnexpected)
	assert.Equal(t, 1, s.n, "n must reset to 1 on transition into extended regime")
	assert.Equal(t, defaultExtendedInitialPollDelay, s.initialDelay, "extended regime initialDelay engaged")
	assert.Equal(t, time.Hour, s.maxDelay, "extended regime maxDelay engaged")

	// First extended-regime wait: T = 5min * 2^0 = 5min, minus jitter in
	// [0, T/2]. Actual wait is in [2.5min, 5min].
	w := s.NextWait()
	assert.GreaterOrEqual(t, w, 2*time.Minute+30*time.Second)
	assert.LessOrEqual(t, w, defaultExtendedInitialPollDelay)
}

// Once in the extended regime, subsequent unexpected failures continue the
// doubling from where n left off -- they do NOT re-reset n to 1. The
// reset-on-transition only fires on the first crossing from normal into
// extended, gated by the inExtended flag.
func TestPollingStrategy_UnexpectedWhileAlreadyExtendedContinuesDoubling(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)

	// Enter extended regime.
	s.OnFailure(FailureClassUnexpected)
	assert.Equal(t, 1, s.n)

	// Second unexpected while already in extended. n advances to 2;
	// initialDelay/maxDelay stay at extended values.
	s.OnFailure(FailureClassUnexpected)
	assert.Equal(t, 2, s.n, "second unexpected in extended increments, does not reset")
	assert.Equal(t, defaultExtendedInitialPollDelay, s.initialDelay)
	assert.Equal(t, time.Hour, s.maxDelay)

	// A normal failure while in extended also advances n without
	// changing regime.
	s.OnFailure(FailureClassNormal)
	assert.Equal(t, 3, s.n, "normal failure in extended increments n")
	assert.Equal(t, defaultExtendedInitialPollDelay, s.initialDelay, "normal failure does not exit extended regime")
	assert.Equal(t, time.Hour, s.maxDelay)
}

// Extended regime doubles per attempt per RETRY §1.4, capped at
// extendedPollMaxDelay (1 hour). Jitter subtracts up to T/2, so each wait falls
// in [T/2, T] before the (in these cases irrelevant) wait floor.
func TestPollingStrategy_ExtendedDoubling(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)

	tests := []struct {
		n                      int
		lowerBound, upperBound time.Duration
	}{
		{1, 2*time.Minute + 30*time.Second, defaultExtendedInitialPollDelay}, // T = 5m
		{2, 5 * time.Minute, 10 * time.Minute},                               // T = 10m
		{3, 10 * time.Minute, 20 * time.Minute},                              // T = 20m
		{4, 20 * time.Minute, 40 * time.Minute},                              // T = 40m
		{5, 30 * time.Minute, time.Hour},                                     // T capped at 60m
		{6, 30 * time.Minute, time.Hour},                                     // still capped
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
// does not clear n or exit the extended regime.
func TestPollingStrategy_FirstSuccessDoesNotReset(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()

	assert.True(t, s.priorPollWasSuccessful, "reset gate should be armed")
	assert.Equal(t, 1, s.n, "one success must not reset n")
	assert.Equal(t, defaultExtendedInitialPollDelay, s.initialDelay, "one success must not exit extended regime")
}

// Two consecutive successes clear n and return the strategy to the
// normal regime (RETRY §1.8 polling binding).
func TestPollingStrategy_TwoConsecutiveSuccessesReset(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()
	s.OnSuccess()

	assert.Equal(t, 0, s.n)
	assert.Equal(t, 30*time.Second, s.initialDelay)
	assert.Equal(t, 30*time.Second, s.maxDelay)
	assert.Equal(t, 30*time.Second, s.NextWait())
}

// A failure between two successes clears the reset gate -- reset requires
// STRICTLY consecutive successes, so a first-then-fail-then-first pattern does
// not fire the reset.
func TestPollingStrategy_FailureClearsResetGate(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()                   // gate armed
	s.OnFailure(FailureClassNormal) // gate cleared, n=2
	s.OnSuccess()                   // gate armed again but does not fire reset

	assert.True(t, s.priorPollWasSuccessful)
	assert.Equal(t, 2, s.n, "reset must not have fired")
}

// A single normal failure after a reset does not re-engage extended-regime
// parameters -- extended engagement requires an Unexpected classification.
func TestPollingStrategy_NormalFailureAfterResetStaysNormal(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)
	// Engage extended, then reset back to normal.
	s.OnFailure(FailureClassUnexpected)
	s.OnSuccess()
	s.OnSuccess()
	// A subsequent normal failure must not re-engage extended.
	s.OnFailure(FailureClassNormal)

	assert.Equal(t, 1, s.n)
	assert.Equal(t, 30*time.Second, s.initialDelay, "normal failure must not engage extended regime")
	assert.Equal(t, 30*time.Second, s.maxDelay)
}

// Regression: when PollInterval equals extendedInitialPollInterval (the default
// combo of 5min/5min was the flagged customer config), the extended-regime
// initialDelay clamps up to equal normalInterval. If transition detection
// relies on that equality, every subsequent unexpected failure re-fires the
// transition path and resets n to 1, and RETRY spec 1.4.1's doubling never
// engages. Explicit regime state (inExtended) avoids the clamp collision.
func TestPollingStrategy_ExtendedDoublingWhenClampedToPollInterval(t *testing.T) {
	s := newPollingStrategy(5*time.Minute, defaultExtendedInitialPollDelay)

	// Drive five unexpected failures; n must advance monotonically.
	for i, expectedN := range []int{1, 2, 3, 4, 5} {
		s.OnFailure(FailureClassUnexpected)
		assert.Equal(t, expectedN, s.n, "failure #%d: n did not advance", i+1)
	}
	assert.Equal(t, 5*time.Minute, s.initialDelay, "initialDelay clamped to PollInterval")
	assert.Equal(t, time.Hour, s.maxDelay, "maxDelay is the extended ceiling")

	// With n=5 the formula T = initialDelay * 2^4 = 80m, clamped to maxDelay=1h.
	// Jitter subtracts up to T/2 = 30m, so wait is in [30m, 1h]. Floor at
	// PollInterval=5m is well below and does not affect the result.
	w := s.NextWait()
	assert.GreaterOrEqual(t, w, 30*time.Minute)
	assert.LessOrEqual(t, w, time.Hour)
}

// OnFailure returns true on the transition into extended, false on every
// subsequent unexpected failure. The caller uses this signal to log
// "engaging extended backoff" exactly once per transition.
func TestPollingStrategy_OnFailureReturnsTrueOnceOnTransition(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)

	assert.True(t, s.OnFailure(FailureClassUnexpected), "first unexpected must signal transition")
	assert.False(t, s.OnFailure(FailureClassUnexpected), "second unexpected must not re-signal transition")
	assert.False(t, s.OnFailure(FailureClassUnexpected), "third unexpected must not re-signal transition")
	assert.False(t, s.OnFailure(FailureClassNormal), "normal failure must not signal transition")
}

// Also test the clamped-config variant, since that's the specific bug scenario
// where equality-based detection re-signalled transition on every unexpected.
func TestPollingStrategy_OnFailureReturnsTrueOnceOnTransitionWhenClamped(t *testing.T) {
	s := newPollingStrategy(5*time.Minute, defaultExtendedInitialPollDelay)

	assert.True(t, s.OnFailure(FailureClassUnexpected), "first unexpected must signal transition")
	assert.False(t, s.OnFailure(FailureClassUnexpected), "second unexpected must not re-signal transition")
	assert.False(t, s.OnFailure(FailureClassUnexpected), "third unexpected must not re-signal transition")
}

// After the two-consecutive-successes reset (RETRY §1.8), the strategy is
// back in the normal regime and a subsequent unexpected failure is treated
// as a new transition, signalling to the caller for a fresh log line.
func TestPollingStrategy_OnSuccessResetAllowsRetransition(t *testing.T) {
	s := newPollingStrategy(30*time.Second, defaultExtendedInitialPollDelay)

	assert.True(t, s.OnFailure(FailureClassUnexpected), "initial transition")
	assert.False(t, s.OnFailure(FailureClassUnexpected), "already in extended")

	// Two consecutive successes: full reset per RETRY §1.8.
	s.OnSuccess()
	s.OnSuccess()

	// Fresh unexpected failure must be treated as a new transition.
	assert.True(t, s.OnFailure(FailureClassUnexpected), "post-reset unexpected must signal a fresh transition")
}

// Case B: PollInterval > extendedInitialPollInterval, but < extendedPollMaxDelay.
// After transition, initialDelay is clamped up to PollInterval so the first
// extended wait equals PollInterval (via the output floor). Doubling engages
// visibly from n=2 and reaches the extended ceiling at n=4.
func TestPollingStrategy_ExtendedDoublingWithModeratePollInterval(t *testing.T) {
	s := newPollingStrategy(10*time.Minute, defaultExtendedInitialPollDelay)

	tests := []struct {
		n                      int
		lowerBound, upperBound time.Duration
	}{
		{1, 10 * time.Minute, 10 * time.Minute}, // T=10m, jitter [0, 5m], wait ∈ [5m, 10m], floor 10m
		{2, 10 * time.Minute, 20 * time.Minute}, // T=20m, wait ∈ [10m, 20m]
		{3, 20 * time.Minute, 40 * time.Minute}, // T=40m, wait ∈ [20m, 40m]
		{4, 30 * time.Minute, time.Hour},        // T=80m -> capped to 1h, wait ∈ [30m, 60m]
		{5, 30 * time.Minute, time.Hour},        // still capped
	}
	for _, tc := range tests {
		s.OnFailure(FailureClassUnexpected)
		w := s.NextWait()
		assert.GreaterOrEqual(t, w, tc.lowerBound, "n=%d lower bound", tc.n)
		assert.LessOrEqual(t, w, tc.upperBound, "n=%d upper bound", tc.n)
	}

	// Confirm the delay bounds after transition:
	assert.Equal(t, 10*time.Minute, s.initialDelay, "initialDelay clamped up to PollInterval")
	assert.Equal(t, time.Hour, s.maxDelay, "maxDelay at extended ceiling")
}

// Case C: PollInterval > extendedPollMaxDelay. Both initialDelay and maxDelay
// are clamped up to PollInterval, so the entire delay range collapses to
// PollInterval and the doubling formula produces no observable variation.
// Extended regime is behaviorally identical to normal regime for this config;
// the transition-log signal still fires but the delay never changes.
func TestPollingStrategy_ExtendedRegimeCollapsesWhenPollIntervalExceedsExtendedCeiling(t *testing.T) {
	s := newPollingStrategy(2*time.Hour, defaultExtendedInitialPollDelay)

	// Multiple unexpected failures -- every wait must be exactly PollInterval.
	for i := 1; i <= 6; i++ {
		s.OnFailure(FailureClassUnexpected)
		assert.Equal(t, 2*time.Hour, s.NextWait(),
			"failure #%d: wait must collapse to PollInterval when PollInterval > extendedPollMaxDelay", i)
	}

	// Both bounds clamped up to PollInterval.
	assert.Equal(t, 2*time.Hour, s.initialDelay, "initialDelay clamped up to PollInterval")
	assert.Equal(t, 2*time.Hour, s.maxDelay, "maxDelay clamped up above extendedPollMaxDelay to PollInterval")
}

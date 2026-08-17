package datasource

import (
	"math"
	"math/rand"
	"time"
)

// extendedPollMaxDelay is the RETRY-spec extended-regime ceiling on the polling
// backoff. Effective ceiling is max(extendedPollMaxDelay, PollInterval); see
// pollingStrategy.OnFailure.
const extendedPollMaxDelay = 1 * time.Hour

// pollingStrategy implements the RETRY §1.4 timing mechanics for the polling
// data source. It owns:
//
//   - the formula-input counter n (RETRY §1.4's "attempts", used as the
//     exponent in T = initialDelay * 2^(n-1); resets on regime transition
//     per RETRY §1.5.3 binding);
//   - the current regime's (initialDelay, maxDelay), toggled by classification
//     (RETRY §1.5--§1.7 via the caller's FailureClass);
//   - the two-consecutive-success reset gate (RETRY §1.8 polling binding);
//   - jitter (RETRY §1.4.3);
//   - the PollInterval wait floor (RETRY §1.4.4 polling override).
//
// All state is owned and mutated by the polling run() goroutine only -- no
// locking required.
type pollingStrategy struct {
	normalInterval              time.Duration
	extendedInitialPollInterval time.Duration
	rng                         *rand.Rand
	n                           int
	priorPollWasSuccessful      bool
	initialDelay                time.Duration
	maxDelay                    time.Duration
	inExtended                  bool
}

func newPollingStrategy(pollInterval, extendedInitialPollInterval time.Duration) *pollingStrategy {
	return &pollingStrategy{
		normalInterval:              pollInterval,
		extendedInitialPollInterval: extendedInitialPollInterval,
		//nolint:gosec // not a cryptographic use-case, weak RNG is acceptable for jitter
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		initialDelay: pollInterval,
		maxDelay:     pollInterval,
	}
}

// OnFailure updates the strategy state after a failed poll. An Unexpected
// classification engages the extended regime for subsequent waits;
// initialDelay is clamped to at least PollInterval so the extended regime
// is never faster than the customer-configured normal cadence.
//
// On the transition from normal into extended regime, n is reset to 1 so
// that the first extended-regime wait uses the new initialDelay. Returns
// true iff this call transitioned the strategy from normal into extended
// regime, so the caller can log a one-time notice.
func (s *pollingStrategy) OnFailure(class FailureClass) (transitionedToExtended bool) {
	s.priorPollWasSuccessful = false
	if class == FailureClassUnexpected && !s.inExtended {
		// transition from normal into extended regime.
		s.n = 1
		s.initialDelay = s.extendedInitialPollInterval
		if s.initialDelay < s.normalInterval {
			s.initialDelay = s.normalInterval
		}
		s.maxDelay = extendedPollMaxDelay
		if s.maxDelay < s.normalInterval {
			s.maxDelay = s.normalInterval
		}
		s.inExtended = true
		return true
	}
	s.n++
	return false
}

// OnSuccess updates the strategy state after a successful poll. Two consecutive
// successes reset n and return the SDK to the normal regime (RETRY §1.8
// polling binding). A single success is a necessary precondition but not
// sufficient -- any failure between the first and second success clears it.
func (s *pollingStrategy) OnSuccess() {
	if s.priorPollWasSuccessful {
		s.n = 0
		s.initialDelay = s.normalInterval
		s.maxDelay = s.normalInterval
		s.inExtended = false
	}
	s.priorPollWasSuccessful = true
}

// NextWait returns the delay before the next poll attempt per RETRY §1.4.
// Formula: T = initialDelay * 2^(n-1), clamped to maxDelay.
// Jitter J is a uniform random in [0, T/2]. The final wait = max(PollInterval,
// T - J) ensures the interval never drops below the caller's configured
// PollInterval.
func (s *pollingStrategy) NextWait() time.Duration {
	if s.n <= 0 {
		return s.normalInterval
	}
	t := time.Duration(math.Min(
		float64(s.initialDelay)*math.Pow(2, float64(s.n-1)),
		float64(s.maxDelay),
	))
	var jitter time.Duration
	if halfT := int64(t / 2); halfT > 0 {
		jitter = time.Duration(s.rng.Int63n(halfT))
	}
	wait := t - jitter
	if wait < s.normalInterval {
		wait = s.normalInterval
	}
	return wait
}

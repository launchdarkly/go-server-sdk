package datasystem

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSynchronizer is a DataSynchronizer whose results are driven entirely by the test.
// A synchronizer that is never fed a result models one that connects and then stays
// silent: it gives the loop nothing to act on, so advancing depends on the fallback
// condition rather than on a reported state.
type fakeSynchronizer struct {
	name      string
	results   chan subsystems.DataSynchronizerResult
	started   chan struct{}
	startOnce sync.Once
}

func newFakeSynchronizer(name string) *fakeSynchronizer {
	return &fakeSynchronizer{
		name:    name,
		results: make(chan subsystems.DataSynchronizerResult, 1),
		started: make(chan struct{}),
	}
}

func (f *fakeSynchronizer) Name() string { return f.name }

func (f *fakeSynchronizer) Fetch(
	_ subsystems.DataSelector,
	_ context.Context,
) (*subsystems.Basis, bool, error) {
	return nil, false, nil
}

func (f *fakeSynchronizer) Sync(_ subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	f.startOnce.Do(func() { close(f.started) })
	return f.results
}

func (f *fakeSynchronizer) Close() error { return nil }

// wasStarted reports whether Sync was called within the timeout.
func (f *fakeSynchronizer) wasStarted(timeout time.Duration) bool {
	select {
	case <-f.started:
		return true
	case <-time.After(timeout):
		return false
	}
}

func makeFallbackTestFDv2(t *testing.T, syncs ...*fakeSynchronizer) *FDv2 {
	t.Helper()
	builders := make([]func() (subsystems.DataSynchronizer, error), 0, len(syncs))
	for _, s := range syncs {
		s := s
		builders = append(builders, func() (subsystems.DataSynchronizer, error) { return s, nil })
	}
	clientContext := &internal.ClientContextImpl{
		BasicClientContext: subsystems.BasicClientContext{
			SDKKey:  sharedtest.TestSDKKey,
			Logging: sharedtest.TestLoggingConfig(),
		},
	}
	system, err := NewFDv2(false, fixedConfig{
		Synchronizers: subsystems.SynchronizersConfiguration{SynchronizerBuilders: builders},
	}, clientContext, nil)
	require.NoError(t, err)
	return system
}

// TestFDv2FallbackConditionDuringStartup covers the predicate that decides whether to
// abandon the current synchronizer. Startup allows a short window and steady state a
// long one, and the two are told apart by whether any data has been applied rather than
// by the status value -- a synchronizer can report Interrupted before the first payload
// arrives, and startup must still use the short window.
func TestFDv2FallbackConditionDuringStartup(t *testing.T) {
	system := makeFallbackTestFDv2(t)
	t.Cleanup(func() { _ = system.Stop() })

	for _, tc := range []struct {
		name        string
		dataApplied bool
		state       interfaces.DataSourceState
		stateAge    time.Duration
		want        bool
	}{
		{
			name:     "starting up, still within the grace period",
			state:    interfaces.DataSourceStateInitializing,
			stateAge: 5 * time.Second,
			want:     false,
		},
		{
			name:     "starting up, past the grace period",
			state:    interfaces.DataSourceStateInitializing,
			stateAge: 11 * time.Second,
			want:     true,
		},
		{
			// Status is Interrupted rather than Initializing because a permanent failure
			// was reported, but no data has ever been applied, so this is still startup
			// and the short window must apply.
			name:     "starting up after a permanent failure, past the grace period",
			state:    interfaces.DataSourceStateInterrupted,
			stateAge: 11 * time.Second,
			want:     true,
		},
		{
			name:     "starting up after a permanent failure, within the grace period",
			state:    interfaces.DataSourceStateInterrupted,
			stateAge: 5 * time.Second,
			want:     false,
		},
		{
			// Once data has been applied the short timer must NOT apply, or a transient
			// blip would cost the current synchronizer after ten seconds instead of a
			// minute, and recovery back to it takes five minutes of healthy operation.
			name:        "running, interrupted for less than a minute",
			dataApplied: true,
			state:       interfaces.DataSourceStateInterrupted,
			stateAge:    11 * time.Second,
			want:        false,
		},
		{
			name:        "running, interrupted for more than a minute",
			dataApplied: true,
			state:       interfaces.DataSourceStateInterrupted,
			stateAge:    61 * time.Second,
			want:        true,
		},
		{
			name:        "running and healthy",
			dataApplied: true,
			state:       interfaces.DataSourceStateValid,
			stateAge:    10 * time.Minute,
			want:        false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			system.dataApplied.Set(tc.dataApplied)
			status := interfaces.DataSourceStatus{
				State:      tc.state,
				StateSince: time.Now().Add(-tc.stateAge),
			}
			assert.Equal(t, tc.want, system.fallbackCond(status))
		})
	}
}

// A synchronizer that reports a reset clears its park escalation.
func TestFDv2ResetSignalClearsParkEscalation(t *testing.T) {
	only := newFakeSynchronizer("only")
	system := makeFallbackTestFDv2(t, only)

	// Stand in for two earlier parks.
	system.synchronizers.slots[0].parkAttempts = 2

	only.results <- subsystems.DataSynchronizerResult{
		State:                interfaces.DataSourceStateValid,
		ChangeSet:            subsystems.NewChangeSetBuilder().Empty(subsystems.NewSelector("state", 1)),
		ResetExtendedBackoff: true,
	}

	ready := make(chan struct{})
	system.Start(ready)
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the result to be consumed")
	}
	require.NoError(t, system.Stop())

	assert.Equal(t, 0, system.synchronizers.slots[0].parkAttempts)
}

// An explicit shutdown is the one condition that still reports Off, matching FDv1. Stop sets the
// status before returning, so no wait is needed here.
func TestFDv2ReportsOffAfterShutdown(t *testing.T) {
	system := makeFallbackTestFDv2(t, newFakeSynchronizer("only"))
	system.Start(make(chan struct{}))

	require.NoError(t, system.Stop())

	assert.Equal(t, interfaces.DataSourceStateOff, system.getStatus().State)
}

// One status can satisfy both the fallback and the recovery condition: Valid for longer than the
// recovery window, with no data ever applied. The data system must recover to the more-preferred
// synchronizer rather than fall back to the next one.
func TestFDv2RecoveryTakesPrecedenceOverFallback(t *testing.T) {
	system := makeFallbackTestFDv2(t, newFakeSynchronizer("first"), newFakeSynchronizer("second"))
	t.Cleanup(func() { _ = system.Stop() })

	// Select the second synchronizer, so that a more-preferred one is eligible.
	system.synchronizers.next()
	system.synchronizers.next()
	require.False(t, system.synchronizers.isFirstEligible())

	status := interfaces.DataSourceStatus{
		State:      interfaces.DataSourceStateValid,
		StateSince: time.Now().Add(-6 * time.Minute),
	}

	// Both conditions hold for this status, which is what makes the precedence observable.
	require.True(t, system.fallbackCond(status))
	require.True(t, system.recoveryCond(status))

	assert.Equal(t, syncRecover, system.conditionAction(status))
}

// After a park the data system waits a jittered second before starting the next synchronizer, so
// that a fleet-wide failure does not restart every instance at the same moment.
func TestFDv2WaitsFailoverDelayAfterParking(t *testing.T) {
	failing := newFakeSynchronizer("failing")
	next := newFakeSynchronizer("next")

	system := makeFallbackTestFDv2(t, failing, next)
	t.Cleanup(func() { _ = system.Stop() })

	failing.results <- subsystems.DataSynchronizerResult{
		State:             interfaces.DataSourceStateInterrupted,
		UnexpectedFailure: true,
		Error: interfaces.DataSourceErrorInfo{
			Kind:       interfaces.DataSourceErrorKindErrorResponse,
			StatusCode: 401,
		},
	}

	system.Start(make(chan struct{}))
	require.True(t, failing.wasStarted(5*time.Second), "first synchronizer should start")

	start := time.Now()
	require.True(t, next.wasStarted(5*time.Second), "second synchronizer should start")
	assert.Greater(t, time.Since(start), 400*time.Millisecond)
}

// TestFDv2ParksSynchronizerOnUnexpectedFailure checks that an unexpected failure moves the
// data system on immediately. The startup fallback window is ten seconds, so a move inside
// two seconds can only have come from the failure itself.
func TestFDv2ParksSynchronizerOnUnexpectedFailure(t *testing.T) {
	failing := newFakeSynchronizer("failing")
	next := newFakeSynchronizer("next")

	system := makeFallbackTestFDv2(t, failing, next)
	t.Cleanup(func() { _ = system.Stop() })

	failing.results <- subsystems.DataSynchronizerResult{
		State:             interfaces.DataSourceStateInterrupted,
		UnexpectedFailure: true,
		Error: interfaces.DataSourceErrorInfo{
			Kind:       interfaces.DataSourceErrorKindErrorResponse,
			StatusCode: 401,
		},
	}

	system.Start(make(chan struct{}))

	require.True(t, failing.wasStarted(5*time.Second), "first synchronizer should start")
	assert.True(t, next.wasStarted(2*time.Second),
		"an unexpected failure should move on without waiting for the fallback condition")
}

// TestFDv2DoesNotReportOffWhileEverySynchronizerIsParked covers the single-synchronizer case,
// where a park leaves nothing running. The condition is transient, because the park expires, so
// the data system must not report a permanent state.
func TestFDv2DoesNotReportOffWhileEverySynchronizerIsParked(t *testing.T) {
	only := newFakeSynchronizer("only")

	system := makeFallbackTestFDv2(t, only)
	t.Cleanup(func() { _ = system.Stop() })

	only.results <- subsystems.DataSynchronizerResult{
		State:             interfaces.DataSourceStateInterrupted,
		UnexpectedFailure: true,
		Error: interfaces.DataSourceErrorInfo{
			Kind:       interfaces.DataSourceErrorKindErrorResponse,
			StatusCode: 401,
		},
	}

	system.Start(make(chan struct{}))
	require.True(t, only.wasStarted(5*time.Second), "the only synchronizer should start")

	assert.Never(t, func() bool {
		return system.DataSourceStatusProvider().GetStatus().State == interfaces.DataSourceStateOff
	}, time.Second, 20*time.Millisecond)
}

// TestFDv2StartupFallsBackPastSilentSynchronizer exercises the whole synchronizer loop
// against the real fallback condition: the first synchronizer reports an unexpected failure
// and the second never reports anything, so reaching the third depends on the startup timer
// still being live after the park.
//
// This runs for longer than most unit tests because it waits out the real ten-second
// startup fallback window twice; the condition's timing is deliberately not stubbed so
// that the test guards the production values.
func TestFDv2StartupFallsBackPastSilentSynchronizer(t *testing.T) {
	failing := newFakeSynchronizer("failing")
	silent := newFakeSynchronizer("silent")
	healthy := newFakeSynchronizer("healthy")

	system := makeFallbackTestFDv2(t, failing, silent, healthy)
	t.Cleanup(func() { _ = system.Stop() })

	// Report an unexpected failure, which parks the first synchronizer.
	failing.results <- subsystems.DataSynchronizerResult{
		State:             interfaces.DataSourceStateInterrupted,
		UnexpectedFailure: true,
		Error: interfaces.DataSourceErrorInfo{
			Kind:       interfaces.DataSourceErrorKindErrorResponse,
			StatusCode: 401,
		},
	}

	system.Start(make(chan struct{}))

	require.True(t, failing.wasStarted(5*time.Second), "first synchronizer should start")
	require.True(t, silent.wasStarted(30*time.Second),
		"should move to the second synchronizer after the permanent failure")
	// The second synchronizer never reports, so only the startup fallback window can
	// advance the loop; the steady-state window is longer than a typical start-wait and
	// would leave the client without data.
	assert.True(t, healthy.wasStarted(30*time.Second),
		"should fall back past the silent synchronizer to the third")
}

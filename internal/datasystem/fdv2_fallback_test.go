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
// A nil results channel models a synchronizer that connects and then stays silent,
// which is the case that exposed the startup fallback bug: a silent source produces no
// result for the loop to act on, so the loop must rely on the fallback condition.
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

// TestFDv2FallbackConditionDuringStartup pins the predicate that decides whether to
// abandon the current synchronizer. The regression this guards: a synchronizer that
// fails permanently during startup leaves the status on Interrupted, so a condition
// keyed on the status being Initializing became unreachable for the rest of startup
// and only the one-minute runtime timer applied -- longer than a typical start-wait.
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
			// The regression. Status is Interrupted rather than Initializing because a
			// permanent failure was reported, but no data has ever been applied, so this
			// is still startup and the short timer must apply.
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

// TestFDv2StartupFallsBackPastSilentSynchronizer exercises the whole synchronizer loop
// against the real fallback condition: the first synchronizer fails permanently and the
// second never reports anything, so reaching the third depends on the startup timer
// still being live after the permanent failure.
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

	// Report a permanent failure, which removes the first synchronizer from the list.
	failing.results <- subsystems.DataSynchronizerResult{
		State: interfaces.DataSourceStateOff,
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
	// advance the loop. Before the fix the status was left on Interrupted and the
	// one-minute runtime timer applied instead, so this is where it hung.
	assert.True(t, healthy.wasStarted(30*time.Second),
		"should fall back past the silent synchronizer to the third")
}

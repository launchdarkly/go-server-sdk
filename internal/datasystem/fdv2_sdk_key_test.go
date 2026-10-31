package datasystem

import (
	"context"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptedSynchronizer is a DataSynchronizer that the test drives. Each call to Sync starts a
// run with its own result channel. The test receives that channel from runs and sends the
// results of the run.
type scriptedSynchronizer struct {
	name string
	runs chan chan<- subsystems.DataSynchronizerResult
}

func newScriptedSynchronizer(name string) *scriptedSynchronizer {
	return &scriptedSynchronizer{name: name, runs: make(chan chan<- subsystems.DataSynchronizerResult, 10)}
}

func (s *scriptedSynchronizer) Name() string { return s.name }

func (s *scriptedSynchronizer) Fetch(
	_ subsystems.DataSelector,
	_ context.Context,
) (*subsystems.Basis, bool, error) {
	return nil, false, nil
}

func (s *scriptedSynchronizer) Sync(_ subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	results := make(chan subsystems.DataSynchronizerResult, 1)
	s.runs <- results
	return results
}

func (s *scriptedSynchronizer) Close() error { return nil }

// requireRun waits for the next run of the synchronizer and returns its result channel.
func (s *scriptedSynchronizer) requireRun(t *testing.T) chan<- subsystems.DataSynchronizerResult {
	t.Helper()
	select {
	case results := <-s.runs:
		return results
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for synchronizer to start", s.name)
		return nil
	}
}

// assertNoRun checks that the synchronizer does not start a run within a short time.
func (s *scriptedSynchronizer) assertNoRun(t *testing.T) {
	t.Helper()
	select {
	case <-s.runs:
		assert.Fail(t, "synchronizer started unexpectedly", s.name)
	case <-time.After(100 * time.Millisecond):
	}
}

var (
	unauthorizedResult = subsystems.DataSynchronizerResult{
		State:             interfaces.DataSourceStateInterrupted,
		Error:             interfaces.DataSourceErrorInfo{Kind: interfaces.DataSourceErrorKindErrorResponse, StatusCode: 401},
		UnexpectedFailure: true,
	}
	validResult = subsystems.DataSynchronizerResult{State: interfaces.DataSourceStateValid}
)

func makeSDKKeyTestFDv2(t *testing.T, syncs ...*scriptedSynchronizer) *FDv2 {
	t.Helper()
	return makeSDKKeyTestFDv2WithFDv1Fallback(t, nil, syncs...)
}

// makeSDKKeyTestFDv2WithFDv1Fallback makes a data system with the synchronizers syncs and the FDv1
// fallback synchronizer fdv1. If fdv1 is nil, no FDv1 fallback is configured.
func makeSDKKeyTestFDv2WithFDv1Fallback(
	t *testing.T,
	fdv1 *scriptedSynchronizer,
	syncs ...*scriptedSynchronizer,
) *FDv2 {
	t.Helper()
	builders := make([]func() (subsystems.DataSynchronizer, error), 0, len(syncs))
	for _, s := range syncs {
		s := s
		builders = append(builders, func() (subsystems.DataSynchronizer, error) { return s, nil })
	}
	var fdv1Builder func() (subsystems.DataSynchronizer, error)
	if fdv1 != nil {
		fdv1Builder = func() (subsystems.DataSynchronizer, error) { return fdv1, nil }
	}
	clientContext := &internal.ClientContextImpl{
		BasicClientContext: subsystems.BasicClientContext{
			SDKKey:  sharedtest.TestSDKKey,
			Logging: sharedtest.TestLoggingConfig(),
		},
	}
	system, err := NewFDv2(false, fixedConfig{
		Synchronizers: subsystems.SynchronizersConfiguration{
			SynchronizerBuilders: builders,
			FDv1FallbackBuilder:  fdv1Builder,
		},
	}, clientContext, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = system.Stop() })
	return system
}

func requireReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for the ready signal")
	}
}

// requireKeyChangeTaken waits until the synchronizer loop has taken the pending key change.
func requireKeyChangeTaken(t *testing.T, system *FDv2) {
	t.Helper()
	require.Eventually(t, func() bool { return len(system.sdkKeyChanged) == 0 }, time.Second, time.Millisecond)
}

func TestFDv2StaysInterruptedWhenEverySynchronizerIsParked(t *testing.T) {
	primary := newScriptedSynchronizer("primary")
	system := makeSDKKeyTestFDv2(t, primary)
	system.Start(make(chan struct{}))

	// The only synchronizer fails, so the data system parks it.
	primary.requireRun(t) <- unauthorizedResult

	// The status is Interrupted with the error of the failure.
	require.Eventually(t, func() bool {
		return system.DataSourceStatusProvider().GetStatus().LastError.StatusCode == 401
	}, time.Second, time.Millisecond)
	assert.Equal(t, interfaces.DataSourceStateInterrupted, system.DataSourceStatusProvider().GetStatus().State)

	// Without a key change, the synchronizer does not start again before its unpark time.
	primary.assertNoRun(t)
}

func TestFDv2KeyChangeRestartsFirstSynchronizerWhenEverySynchronizerFailed(t *testing.T) {
	primary := newScriptedSynchronizer("primary")
	secondary := newScriptedSynchronizer("secondary")
	system := makeSDKKeyTestFDv2(t, primary, secondary)
	system.Start(make(chan struct{}))

	// Both synchronizers fail, so the data system parks both.
	primary.requireRun(t) <- unauthorizedResult
	secondary.requireRun(t) <- unauthorizedResult

	// After a key change, the primary synchronizer starts again and gets data. The secondary
	// synchronizer does not start.
	system.SDKKeyChanged()
	primary.requireRun(t) <- validResult
	assert.True(t, system.DataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second))
	secondary.assertNoRun(t)
}

func TestFDv2KeyChangeRestartsParkedFDv1FallbackSynchronizer(t *testing.T) {
	// The primary synchronizer directs a fallback to FDv1. Then the FDv1 fallback synchronizer
	// fails, so the data system parks it.
	primary := newScriptedSynchronizer("primary")
	fdv1 := newScriptedSynchronizer("fdv1")
	system := makeSDKKeyTestFDv2WithFDv1Fallback(t, fdv1, primary)
	system.Start(make(chan struct{}))
	primary.requireRun(t) <- subsystems.DataSynchronizerResult{
		State:          interfaces.DataSourceStateInterrupted,
		FallbackToFDv1: true,
	}
	fdv1.requireRun(t) <- unauthorizedResult

	// After a key change, the FDv1 fallback synchronizer starts again and gets data. The data
	// system stays on FDv1, so the primary synchronizer does not start.
	system.SDKKeyChanged()
	fdv1.requireRun(t) <- validResult
	assert.True(t, system.DataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second))
	primary.assertNoRun(t)
}

func TestFDv2KeyChangeDuringRunReturnsToFirstSynchronizerAfterFailure(t *testing.T) {
	// The first synchronizer fails and is parked, so the second synchronizer runs.
	first := newScriptedSynchronizer("first")
	second := newScriptedSynchronizer("second")
	third := newScriptedSynchronizer("third")
	system := makeSDKKeyTestFDv2(t, first, second, third)
	system.Start(make(chan struct{}))
	first.requireRun(t) <- unauthorizedResult
	secondRun := second.requireRun(t)

	// The key changes while the second synchronizer runs.
	system.SDKKeyChanged()
	requireKeyChangeTaken(t, system)

	// The second synchronizer then fails. The data system does not park it. It returns to the
	// first synchronizer, which the key change unparked, rather than moving to the third.
	secondRun <- unauthorizedResult
	first.requireRun(t)
	third.assertNoRun(t)
	second.assertNoRun(t)
}

func TestFDv2KeyChangeDoesNotInterruptRunningSynchronizer(t *testing.T) {
	// The primary synchronizer fails and is parked, so the secondary synchronizer runs and gets data.
	primary := newScriptedSynchronizer("primary")
	secondary := newScriptedSynchronizer("secondary")
	system := makeSDKKeyTestFDv2(t, primary, secondary)
	system.Start(make(chan struct{}))
	primary.requireRun(t) <- unauthorizedResult
	secondary.requireRun(t) <- validResult

	// The key change unparks the primary synchronizer, but the secondary synchronizer keeps
	// running. The recovery condition decides when to return to the primary synchronizer.
	system.SDKKeyChanged()
	requireKeyChangeTaken(t, system)
	primary.assertNoRun(t)
	assert.Equal(t, interfaces.DataSourceStateValid, system.DataSourceStatusProvider().GetStatus().State)
}

func TestFDv2KeyChangeDoesNotUndoFallbackDirectiveWithoutFDv1Fallback(t *testing.T) {
	// No FDv1 fallback is configured, so a fallback directive stops every synchronizer until the TTL elapses.
	primary := newScriptedSynchronizer("primary")
	system := makeSDKKeyTestFDv2(t, primary)
	system.Start(make(chan struct{}))
	primary.requireRun(t) <- subsystems.DataSynchronizerResult{
		State:          interfaces.DataSourceStateInterrupted,
		FallbackToFDv1: true,
	}
	require.Eventually(t, func() bool {
		return system.DataSourceStatusProvider().GetStatus().State == interfaces.DataSourceStateInterrupted
	}, time.Second, time.Millisecond)

	// A key change does not start a synchronizer, and the data system stays interrupted.
	system.SDKKeyChanged()
	requireKeyChangeTaken(t, system)
	primary.assertNoRun(t)
	assert.Equal(t, interfaces.DataSourceStateInterrupted, system.DataSourceStatusProvider().GetStatus().State)
}

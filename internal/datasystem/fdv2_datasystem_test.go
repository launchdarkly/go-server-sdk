package datasystem

import (
	"sync"
	"testing"
	"time"

	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

func TestFDv2WaitForReturnsAfterTimeoutWhileStatusUpdatesAreQueued(t *testing.T) {
	system := makeFallbackTestFDv2(t)
	t.Cleanup(func() { _ = system.Stop() })
	provider := &dataStatusProvider{system: system}

	// Flood the status broadcaster so that, when the wait times out, broadcasts wait on
	// the full listener channel. The removal of the listener inside WaitFor must still
	// complete.
	stop := make(chan struct{})
	var flooders sync.WaitGroup
	for i := 0; i < 8; i++ {
		flooders.Add(1)
		go func() {
			defer flooders.Done()
			status := interfaces.DataSourceStatus{State: interfaces.DataSourceStateInterrupted}
			for {
				select {
				case <-stop:
					return
				default:
					system.broadcasters.dataSourceStatus.Broadcast(status)
				}
			}
		}()
	}

	returned := make(chan bool, 1)
	go func() {
		returned <- provider.WaitFor(interfaces.DataSourceStateValid, 50*time.Millisecond)
	}()
	select {
	case success := <-returned:
		assert.False(t, success)
	case <-time.After(2 * time.Second):
		t.Error("WaitFor did not return after its timeout")
	}
	close(stop)
	flooders.Wait()
}

func TestFDv2WaitForEndsWhenStopped(t *testing.T) {
	system := makeFallbackTestFDv2(t)
	provider := &dataStatusProvider{system: system}
	go func() {
		<-time.After(100 * time.Millisecond)
		_ = system.Stop()
	}()

	// Wait for a state that never arrives.
	timeStart := time.Now()
	success := provider.WaitFor(interfaces.DataSourceStateValid, 500*time.Millisecond)

	// WaitFor should return false when the system stops, before the timeout.
	assert.False(t, success)
	assert.Less(t, time.Since(timeStart), 500*time.Millisecond)
}

func TestFDv2GetStatusReturnsWhileStatusListenerIsFull(t *testing.T) {
	system := makeFallbackTestFDv2(t)
	t.Cleanup(func() { _ = system.Stop() })
	provider := &dataStatusProvider{system: system}
	listener := provider.AddStatusListener()
	t.Cleanup(func() { provider.RemoveStatusListener(listener) })

	// Fill the listener's 10-value buffer, then block one more status update on it.
	for i := 0; i < 10; i++ {
		system.UpdateStatus(interfaces.DataSourceStateInterrupted, interfaces.DataSourceErrorInfo{StatusCode: 500 + i})
	}
	go system.UpdateStatus(interfaces.DataSourceStateInterrupted, interfaces.DataSourceErrorInfo{StatusCode: 510})
	time.Sleep(10 * time.Millisecond)

	// GetStatus should not wait for the listener.
	returned := make(chan struct{})
	go func() {
		provider.GetStatus()
		close(returned)
	}()
	th.AssertChannelClosed(t, returned, time.Second)
}

func TestFDv2StopReturnsWhileStatusListenerIsFull(t *testing.T) {
	system := makeFallbackTestFDv2(t)
	provider := &dataStatusProvider{system: system}
	listener := provider.AddStatusListener()
	t.Cleanup(func() { provider.RemoveStatusListener(listener) })

	// Fill the listener's 10-value buffer.
	for i := 0; i < 10; i++ {
		system.UpdateStatus(interfaces.DataSourceStateInterrupted, interfaces.DataSourceErrorInfo{StatusCode: 500 + i})
	}

	// Stop without reading from the listener.
	stopped := make(chan struct{})
	go func() {
		_ = system.Stop()
		close(stopped)
	}()

	// Stop should return, and the status should be Off.
	if th.AssertChannelClosed(t, stopped, time.Second) {
		assert.Equal(t, interfaces.DataSourceStateOff, provider.GetStatus().State)
	}
}

func TestFDv2SynchronizerSignalsReadinessOnlyForAFullPayload(t *testing.T) {
	primary := newScriptedSynchronizer("primary")
	system := makeSDKKeyTestFDv2(t, primary)
	ready := make(chan struct{})
	system.Start(ready)
	run := primary.requireRun(t)

	// An up-to-date result carries no data, so the data system is not ready.
	run <- subsystems.DataSynchronizerResult{
		State:     interfaces.DataSourceStateValid,
		ChangeSet: subsystems.NewChangeSetBuilder().NoChanges(),
	}
	select {
	case <-ready:
		assert.Fail(t, "ready after an up-to-date result")
	case <-time.After(100 * time.Millisecond):
	}

	// A full payload makes the data system ready.
	run <- subsystems.DataSynchronizerResult{
		State:     interfaces.DataSourceStateValid,
		ChangeSet: subsystems.NewChangeSetBuilder().Empty(subsystems.NewSelector("state", 1)),
	}
	requireReady(t, ready)
}

package datasystem

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
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

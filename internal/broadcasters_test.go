package internal

import (
	"fmt"
	"sync"
	"testing"
	"time"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
)

func TestBroadcaster(t *testing.T) {
	var n int
	testBroadcasterGenerically(t, NewBroadcaster[string],
		func() string {
			n += 1
			return fmt.Sprintf("value%d", n)
		})
}

func testBroadcasterGenerically[V any](t *testing.T, broadcasterFactory func() *Broadcaster[V], valueFactory func() V) {
	timeout := time.Second

	withBroadcaster := func(t *testing.T, action func(*Broadcaster[V])) {
		b := broadcasterFactory()
		defer b.Close()
		action(b)
	}

	t.Run("broadcast with no subscribers", func(t *testing.T) {
		withBroadcaster(t, func(b *Broadcaster[V]) {
			b.Broadcast(valueFactory())
		})
	})

	t.Run("broadcast with subscribers", func(t *testing.T) {
		withBroadcaster(t, func(b *Broadcaster[V]) {
			ch1 := b.AddListener()
			ch2 := b.AddListener()

			value := valueFactory()
			b.Broadcast(value)

			assert.Equal(t, value, th.RequireValue(t, ch1, timeout))
			assert.Equal(t, value, th.RequireValue(t, ch2, timeout))
		})
	})

	t.Run("broadcast to full listener delivers every value", func(t *testing.T) {
		withBroadcaster(t, func(b *Broadcaster[V]) {
			ch := b.AddListener()
			values := make([]V, subscriberChannelBufferLength+1)
			for i := range values {
				values[i] = valueFactory()
			}

			// Broadcast one more value than the channel can hold.
			go func() {
				for _, v := range values {
					b.Broadcast(v)
				}
			}()

			// The listener should receive every value in order.
			for _, v := range values {
				assert.Equal(t, v, th.RequireValue(t, ch, timeout))
			}
		})
	})

	t.Run("unregister subscriber", func(t *testing.T) {
		withBroadcaster(t, func(b *Broadcaster[V]) {
			ch1 := b.AddListener()
			ch2 := b.AddListener()

			b.RemoveListener(ch1)
			th.AssertChannelClosed(t, ch1, time.Millisecond)

			value := valueFactory()
			b.Broadcast(value)

			assert.Equal(t, value, th.RequireValue(t, ch2, timeout))
		})
	})

	unblockTests := []struct {
		name    string
		release func(b *Broadcaster[V], ch <-chan V)
	}{
		{"remove listener unblocks broadcast to full listener",
			func(b *Broadcaster[V], ch <-chan V) { b.RemoveListener(ch) }},
		{"close unblocks broadcast to full listener",
			func(b *Broadcaster[V], _ <-chan V) { b.Close() }},
	}
	for _, tt := range unblockTests {
		t.Run(tt.name, func(t *testing.T) {
			withBroadcaster(t, func(b *Broadcaster[V]) {
				ch := b.AddListener()
				for i := 0; i < subscriberChannelBufferLength; i++ {
					b.Broadcast(valueFactory())
				}

				broadcastDone := make(chan struct{})
				go func() {
					b.Broadcast(valueFactory())
					close(broadcastDone)
				}()
				// Give Broadcast time to block on the full channel.
				time.Sleep(10 * time.Millisecond)

				// Release the listener without reading from it.
				releaseDone := make(chan struct{})
				go func() {
					tt.release(b, ch)
					close(releaseDone)
				}()

				// Both calls should return.
				ok := th.AssertChannelClosed(t, broadcastDone, timeout) &&
					th.AssertChannelClosed(t, releaseDone, timeout)
				if !ok {
					<-ch
				}
			})
		})
	}

	t.Run("hasListeners", func(t *testing.T) {
		withBroadcaster(t, func(b *Broadcaster[V]) {
			assert.False(t, b.HasListeners())

			ch1 := b.AddListener()
			ch2 := b.AddListener()

			assert.True(t, b.HasListeners())

			b.RemoveListener(ch1)

			assert.True(t, b.HasListeners())

			b.RemoveListener(ch2)

			assert.False(t, b.HasListeners())
		})
	})
}

func TestBroadcasterDataRace(t *testing.T) {
	t.Parallel()
	b := NewBroadcaster[string]()
	t.Cleanup(b.Close)

	var waitGroup sync.WaitGroup
	for _, fn := range []func(){
		// Run every method that uses b.subscribers concurrently to detect data races
		func() { b.AddListener() },
		func() { b.Broadcast("foo") },
		func() { b.Close() },
		func() { b.HasListeners() },
		func() { b.RemoveListener(nil) },
	} {
		const concurrentRoutinesWithSelf = 2
		// Run a method concurrently with itself to detect data races. These methods will also be
		// run concurrently with the previous/next methods in the list.
		for i := 0; i < concurrentRoutinesWithSelf; i++ {
			waitGroup.Add(1)
			fn := fn // make fn a loop-local variable
			go func() {
				defer waitGroup.Done()
				fn()
			}()
		}
	}
	waitGroup.Wait()
}

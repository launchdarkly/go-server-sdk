package internal

import (
	"fmt"
	"math/rand"
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

func TestBroadcasterDataRaceSelf(t *testing.T) {
	t.Parallel()
	b := NewBroadcaster[string]()
	t.Cleanup(b.Close)

	var waitGroup sync.WaitGroup
	for _, fn := range []func(){
		// run every method that uses b.subscribers concurrently to detect data races
		func() { b.AddListener() },
		func() { b.Broadcast("foo") },
		func() { b.Close() },
		func() { b.HasListeners() },
		func() { b.RemoveListener(nil) },
	} {
		const concurrentRoutinesWithSelf = 10
		// run a method concurrently with itself to detect data races
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

func TestBroadcasterDataRaceRandomFunctions(t *testing.T) {
	t.Parallel()
	b := NewBroadcaster[string]()
	t.Cleanup(b.Close)

	funcs := []func(){
		func() { b.AddListener() },
		func() { b.Broadcast("foo") },
		func() { b.Close() },
		func() { b.HasListeners() },
		func() { b.RemoveListener(nil) },
	}
	var waitGroup sync.WaitGroup

	const N = 1000

	// We're going to keep adding random functions to the set of currently executing functions
	// for N iterations. This way, we can detect races between different methods, or those that are only caused
	// by a particular execution order.

	for i := 0; i < N; i++ {
		waitGroup.Add(1)
		fn := funcs[rand.Intn(len(funcs))]
		go func() {
			defer waitGroup.Done()
			fn()
		}()
	}
	waitGroup.Wait()
}

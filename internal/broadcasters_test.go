package internal

import (
	"fmt"
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

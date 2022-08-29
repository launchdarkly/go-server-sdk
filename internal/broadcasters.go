package internal

import (
	"sync"

	"golang.org/x/exp/slices"
)

// This file defines the publish-subscribe model we use for various status/event types in the SDK.
//
// The standard pattern is that AddListener returns a new receive-only channel; RemoveListener unsubscribes
// that channel, and closes the sending end of it; Broadcast sends a value to all of the subscribed channels
// (if any); and Close unsubscribes and closes all existing channels.

// Arbitrary buffer size to make it less likely that we'll block when broadcasting to channels. It is still
// the consumer's responsibility to make sure they're reading the channel.
const subscriberChannelBufferLength = 10

// Broadcaster is our generalized implementation of broadcasters.
type Broadcaster[V any] struct {
	subscribers []channelPair[V]
	lock        sync.Mutex
}

// We need to keep track of both the channel we use for sending (stored as a reflect.Value, because Value
// has methods for sending and closing), and also the
type channelPair[V any] struct {
	sendCh    chan<- V
	receiveCh <-chan V
}

// NewBroadcaster creates a Broadcaster that operates on the specified value type.
func NewBroadcaster[V any]() *Broadcaster[V] {
	return &Broadcaster[V]{}
}

// AddListener adds a subscriber and returns a channel for it to receive values.
func (b *Broadcaster[V]) AddListener() <-chan V {
	ch := make(chan V, subscriberChannelBufferLength)
	var receiveCh <-chan V = ch
	chPair := channelPair[V]{sendCh: ch, receiveCh: receiveCh}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.subscribers = append(b.subscribers, chPair)
	return receiveCh
}

// RemoveListener removes a subscriber. The parameter is the same channel that was returned by
// AddListener.
func (b *Broadcaster[V]) RemoveListener(ch <-chan V) {
	b.lock.Lock()
	defer b.lock.Unlock()
	ss := b.subscribers
	for i, s := range ss {
		// The following equality test is the reason why we have to store both the sendCh (chan X) and
		// the receiveCh (<-chan X) for each subscriber; "s.sendCh == ch" would not be true because
		// they're of two different types.
		if s.receiveCh == ch {
			copy(ss[i:], ss[i+1:])
			ss[len(ss)-1] = channelPair[V]{}
			b.subscribers = ss[:len(ss)-1]
			close(s.sendCh)
			break
		}
	}
}

// HasListeners returns true if there are any current subscribers.
func (b *Broadcaster[V]) HasListeners() bool {
	return len(b.subscribers) > 0
}

// Broadcast broadcasts a value to all current subscribers.
func (b *Broadcaster[V]) Broadcast(value V) {
	b.lock.Lock()
	ss := slices.Clone(b.subscribers)
	b.lock.Unlock()
	if len(ss) > 0 {
		for _, ch := range ss {
			ch.sendCh <- value
		}
	}
}

// Close closes all current subscriber channels.
func (b *Broadcaster[V]) Close() {
	b.lock.Lock()
	defer b.lock.Unlock()
	for _, s := range b.subscribers {
		close(s.sendCh)
	}
	b.subscribers = nil
}

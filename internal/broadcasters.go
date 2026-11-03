package internal

import (
	"sync"

	"slices"
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
	// Guarded by lock. Do not hold lock while sending to a subscriber.
	subscribers []*channelPair[V]
	lock        sync.RWMutex
}

// We need to keep track of both the channel we use for sending and the channel for receiving.
type channelPair[V any] struct {
	sendCh    chan<- V
	receiveCh <-chan V

	// Closed when the subscriber is removed. A send to sendCh must stop when done is closed.
	done chan struct{}

	// Sending to sendCh requires lock (read). Closing sendCh requires lock (write), after done is closed.
	lock sync.RWMutex
}

// send sends value to sendCh, unless the subscriber is removed first.
func (p *channelPair[V]) send(value V) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	// Check that it's not done first, so we don't send on a closed channel.
	select {
	case <-p.done:
		return
	default:
	}
	select {
	case p.sendCh <- value:
	case <-p.done:
	}
}

// close stops any send in progress and closes sendCh. Call it once per subscriber.
func (p *channelPair[V]) close() {
	close(p.done)
	p.lock.Lock()
	defer p.lock.Unlock()
	close(p.sendCh)
}

// NewBroadcaster creates a Broadcaster that operates on the specified value type.
func NewBroadcaster[V any]() *Broadcaster[V] {
	return &Broadcaster[V]{}
}

// AddListener adds a subscriber and returns a channel for it to receive values.
func (b *Broadcaster[V]) AddListener() <-chan V {
	ch := make(chan V, subscriberChannelBufferLength)
	var receiveCh <-chan V = ch
	chPair := &channelPair[V]{sendCh: ch, receiveCh: receiveCh, done: make(chan struct{})}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.subscribers = append(b.subscribers, chPair)
	return receiveCh
}

// RemoveListener removes a subscriber. The parameter is the same channel that was returned by
// AddListener.
func (b *Broadcaster[V]) RemoveListener(ch <-chan V) {
	var removed []*channelPair[V]
	b.lock.Lock()
	b.subscribers = slices.DeleteFunc(b.subscribers, func(pair *channelPair[V]) bool {
		// The following equality test is the reason why we have to store both the sendCh (chan X) and
		// the receiveCh (<-chan X) for each subscriber; "s.sendCh == ch" would not be true because
		// they're of two different types.
		if pair.receiveCh == ch {
			removed = append(removed, pair)
			return true
		}
		return false
	})
	b.lock.Unlock()
	for _, pair := range removed {
		pair.close()
	}
}

// HasListeners returns true if there are any current subscribers.
func (b *Broadcaster[V]) HasListeners() bool {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return len(b.subscribers) > 0
}

// Broadcast broadcasts a value to all current subscribers.
func (b *Broadcaster[V]) Broadcast(value V) {
	b.lock.RLock()
	subscribers := slices.Clone(b.subscribers)
	b.lock.RUnlock()
	for _, pair := range subscribers {
		pair.send(value)
	}
}

// Close closes all current subscriber channels.
func (b *Broadcaster[V]) Close() {
	b.lock.Lock()
	subscribers := b.subscribers
	b.subscribers = nil
	b.lock.Unlock()
	for _, pair := range subscribers {
		pair.close()
	}
}

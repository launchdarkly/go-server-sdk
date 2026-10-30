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
//
// Lock order: acquire releaseLock before lock.
type Broadcaster[V any] struct {
	// Reading subscribers requires lock (read or write) or releaseLock.
	// Modifying subscribers requires both releaseLock and lock (write).
	subscribers []channelPair[V]

	// Sending to a subscriber requires lock (read).
	lock sync.RWMutex

	// Closing a subscriber's done channel requires releaseLock.
	// Do not hold releaseLock while sending to a subscriber.
	releaseLock sync.Mutex
}

// We need to keep track of both the channel we use for sending and the channel for receiving.
type channelPair[V any] struct {
	sendCh    chan<- V
	receiveCh <-chan V

	// A send to sendCh must stop when done is closed.
	done chan struct{}
}

// NewBroadcaster creates a Broadcaster that operates on the specified value type.
func NewBroadcaster[V any]() *Broadcaster[V] {
	return &Broadcaster[V]{}
}

// AddListener adds a subscriber and returns a channel for it to receive values.
func (b *Broadcaster[V]) AddListener() <-chan V {
	ch := make(chan V, subscriberChannelBufferLength)
	var receiveCh <-chan V = ch
	chPair := channelPair[V]{sendCh: ch, receiveCh: receiveCh, done: make(chan struct{})}
	b.releaseLock.Lock()
	defer b.releaseLock.Unlock()
	b.lock.Lock()
	defer b.lock.Unlock()
	b.subscribers = append(b.subscribers, chPair)
	return receiveCh
}

// RemoveListener removes a subscriber. The parameter is the same channel that was returned by
// AddListener.
func (b *Broadcaster[V]) RemoveListener(ch <-chan V) {
	b.releaseLock.Lock()
	defer b.releaseLock.Unlock()
	for _, pair := range b.subscribers {
		if pair.receiveCh == ch {
			close(pair.done)
		}
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.subscribers = slices.DeleteFunc(b.subscribers, func(pair channelPair[V]) bool {
		// The following equality test is the reason why we have to store both the sendCh (chan X) and
		// the receiveCh (<-chan X) for each subscriber; "s.sendCh == ch" would not be true because
		// they're of two different types.
		if pair.receiveCh == ch {
			close(pair.sendCh)
			return true
		}
		return false
	})
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
	defer b.lock.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch.sendCh <- value:
		case <-ch.done:
		}
	}
}

// Close closes all current subscriber channels.
func (b *Broadcaster[V]) Close() {
	b.releaseLock.Lock()
	defer b.releaseLock.Unlock()
	for _, s := range b.subscribers {
		close(s.done)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	for _, s := range b.subscribers {
		close(s.sendCh)
	}
	b.subscribers = nil
}

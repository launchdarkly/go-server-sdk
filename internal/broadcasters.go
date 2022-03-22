package internal

import (
	"reflect"
	"sync"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
)

// This file contains all of the types we use for the publish-subscribe model for various status types. The
// core logic is implemented using reflection in the genericBroadcaster type; the specialized types that
// wrap genericBroadcaster just enforce that we're using them with the correct status type.
//
// The standard pattern is that AddListener returns a new receive-only channel; RemoveListener unsubscribes
// that channel, and closes the sending end of it; Broadcast sends a value to all of the subscribed channels
// (if any); and Close unsubscribes and closes all existing channels.

// Any type added here *must* have a corresponding unit test suite in broadcasters_test.go using the same
// pattern as the other tests there.

// Arbitrary buffer size to make it less likely that we'll block when broadcasting to channels. It is still
// the consumer's responsibility to make sure they're reading the channel.
const subscriberChannelBufferLength = 10

// DataStoreStatusBroadcaster is the internal implementation of publish-subscribe for DataStoreStatus values.
type DataStoreStatusBroadcaster struct {
	g *genericBroadcaster
}

// NewDataStoreStatusBroadcaster creates an instance of DataStoreStatusBroadcaster.
func NewDataStoreStatusBroadcaster() *DataStoreStatusBroadcaster {
	return &DataStoreStatusBroadcaster{
		g: newGenericBroadcaster(chan interfaces.DataStoreStatus(nil), (<-chan interfaces.DataStoreStatus)(nil)),
	}
}

// AddListener creates a new channel for listening to broadcast values. This is created with a small
// channel buffer, but it is the consumer's responsibility to consume the channel to avoid blocking an
// SDK goroutine.
func (b *DataStoreStatusBroadcaster) AddListener() <-chan interfaces.DataStoreStatus {
	ch, _ := b.g.addListenerInternal().(<-chan interfaces.DataStoreStatus)
	return ch
}

// RemoveListener stops broadcasting to a channel that was created with AddListener.
func (b *DataStoreStatusBroadcaster) RemoveListener(ch <-chan interfaces.DataStoreStatus) {
	b.g.removeListenerInternal(ch)
}

// Broadcast broadcasts a new value to the registered listeners, if any.
func (b *DataStoreStatusBroadcaster) Broadcast(value interfaces.DataStoreStatus) {
	b.g.broadcastInternal(value)
}

// Close closes all currently registered listener channels.
func (b *DataStoreStatusBroadcaster) Close() { b.g.close() }

// DataSourceStatusBroadcaster is the internal implementation of publish-subscribe for DataSourceStatus values.
type DataSourceStatusBroadcaster struct {
	g *genericBroadcaster
}

// NewDataSourceStatusBroadcaster creates an instance of DataSourceStatusBroadcaster.
func NewDataSourceStatusBroadcaster() *DataSourceStatusBroadcaster {
	return &DataSourceStatusBroadcaster{
		g: newGenericBroadcaster(chan interfaces.DataSourceStatus(nil), (<-chan interfaces.DataSourceStatus)(nil)),
	}
}

// AddListener creates a new channel for listening to broadcast values. This is created with a small
// channel buffer, but it is the consumer's responsibility to consume the channel to avoid blocking an
// SDK goroutine.
func (b *DataSourceStatusBroadcaster) AddListener() <-chan interfaces.DataSourceStatus {
	ch, _ := b.g.addListenerInternal().(<-chan interfaces.DataSourceStatus)
	return ch
}

// RemoveListener stops broadcasting to a channel that was created with AddListener.
func (b *DataSourceStatusBroadcaster) RemoveListener(ch <-chan interfaces.DataSourceStatus) {
	b.g.removeListenerInternal(ch)
}

// Broadcast broadcasts a new value to the registered listeners, if any.
func (b *DataSourceStatusBroadcaster) Broadcast(value interfaces.DataSourceStatus) {
	b.g.broadcastInternal(value)
}

// Close closes all currently registered listener channels.
func (b *DataSourceStatusBroadcaster) Close() { b.g.close() }

// FlagChangeEventBroadcaster is the internal implementation of publish-subscribe for FlagChangeEvent values.
type FlagChangeEventBroadcaster struct {
	g *genericBroadcaster
}

// NewFlagChangeEventBroadcaster creates an instance of FlagChangeEventBroadcaster.
func NewFlagChangeEventBroadcaster() *FlagChangeEventBroadcaster {
	return &FlagChangeEventBroadcaster{
		g: newGenericBroadcaster(chan interfaces.FlagChangeEvent(nil), (<-chan interfaces.FlagChangeEvent)(nil)),
	}
}

// AddListener creates a new channel for listening to broadcast values. This is created with a small
// channel buffer, but it is the consumer's responsibility to consume the channel to avoid blocking an
// SDK goroutine.
func (b *FlagChangeEventBroadcaster) AddListener() <-chan interfaces.FlagChangeEvent {
	ch, _ := b.g.addListenerInternal().(<-chan interfaces.FlagChangeEvent)
	return ch
}

// RemoveListener stops broadcasting to a channel that was created with AddListener.
func (b *FlagChangeEventBroadcaster) RemoveListener(ch <-chan interfaces.FlagChangeEvent) {
	b.g.removeListenerInternal(ch)
}

// HasListeners returns true if any listeners are registered.
func (b *FlagChangeEventBroadcaster) HasListeners() bool {
	return b.g.hasListeners()
}

// Broadcast broadcasts a new value to the registered listeners, if any.
func (b *FlagChangeEventBroadcaster) Broadcast(value interfaces.FlagChangeEvent) {
	b.g.broadcastInternal(value)
}

// Close closes all currently registered listener channels.
func (b *FlagChangeEventBroadcaster) Close() { b.g.close() }

// BigSegmentStoreStatusBroadcaster is the internal implementation of publish-subscribe for
// BigSegmentStoreStatus values.
type BigSegmentStoreStatusBroadcaster struct {
	g *genericBroadcaster
}

// NewBigSegmentStoreStatusBroadcaster creates an instance of BigSegmentStoreStatusBroadcaster.
func NewBigSegmentStoreStatusBroadcaster() *BigSegmentStoreStatusBroadcaster {
	return &BigSegmentStoreStatusBroadcaster{
		g: newGenericBroadcaster(chan interfaces.BigSegmentStoreStatus(nil),
			(<-chan interfaces.BigSegmentStoreStatus)(nil)),
	}
}

// AddListener creates a new channel for listening to broadcast values. This is created with a small
// channel buffer, but it is the consumer's responsibility to consume the channel to avoid blocking an
// SDK goroutine.
func (b *BigSegmentStoreStatusBroadcaster) AddListener() <-chan interfaces.BigSegmentStoreStatus {
	ch, _ := b.g.addListenerInternal().(<-chan interfaces.BigSegmentStoreStatus)
	return ch
}

// RemoveListener stops broadcasting to a channel that was created with AddListener.
func (b *BigSegmentStoreStatusBroadcaster) RemoveListener(ch <-chan interfaces.BigSegmentStoreStatus) {
	b.g.removeListenerInternal(ch)
}

// Broadcast broadcasts a new value to the registered listeners, if any.
func (b *BigSegmentStoreStatusBroadcaster) Broadcast(value interfaces.BigSegmentStoreStatus) {
	b.g.broadcastInternal(value)
}

// Close closes all currently registered listener channels.
func (b *BigSegmentStoreStatusBroadcaster) Close() { b.g.close() }

// genericBroadcaster is our reflection-based generalized implementation of broadcasters.
type genericBroadcaster struct {
	channelType        reflect.Type
	receiveChannelType reflect.Type
	elementType        reflect.Type
	subscribers        []genericChannelPair
	lock               sync.Mutex
}

// We need to keep track of both the channel we use for sending (stored as a reflect.Value, because Value
// has methods for sending and closing), and also the
type genericChannelPair struct {
	sendCh    reflect.Value
	receiveCh interface{}
}

// Creates a genericBroadcaster that operates on the specified type of channel. In all of the following
// comments, let's say that X is the type of value or event being sent by the broadcaster.
//
// exampleChannel: this must have the type "chan X". An actual channel is not required; it can be nil,
//   for instance:  chan X(nil)
// exampleReceiveChannel: same thing, except this is the "<-chan" version of the type (which is what we return
//   from AddListener). To specify a nil value of this, be careful to use (<-chan X)(nil) and not <-chan X(nil)
//   because the latter is actually a receive statement.
//
// If the types do not match this pattern in any way, the constructor will return a stub genericBroadcaster
// that creates no channels and sends no values, so that it cannot cause typecasting-related panics.
func newGenericBroadcaster(exampleChannel interface{}, exampleReceiveChannel interface{}) *genericBroadcaster {
	b := &genericBroadcaster{
		channelType:        reflect.TypeOf(exampleChannel),
		receiveChannelType: reflect.TypeOf(exampleReceiveChannel),
	}
	if b.channelType.Kind() != reflect.Chan || b.channelType.ChanDir() != reflect.BothDir {
		return &genericBroadcaster{}
	}
	if b.receiveChannelType.Kind() != reflect.Chan || b.receiveChannelType.ChanDir() != reflect.RecvDir {
		return &genericBroadcaster{}
	}
	if !b.channelType.ConvertibleTo(b.receiveChannelType) {
		return &genericBroadcaster{}
	}
	b.elementType = b.channelType.Elem()
	return b
}

// Reflective implementation of AddListener. The return value is really of type <-chan X and can always
// be safely cast to that type.
//
// If it cannot create a channel because the genericBroadcaster was configured with an invalid type, it
// returns a nil channel-- so the type-specific broadcaster's AddListener method will also return nil.
// This should never happen; our unit test suite verifies that every broadcaster type correctly configures
// itself and can create listeners. But if it somehow did happen, attempting to receive from a nil channel
// would not cause a panic; it just would not receive anything.
func (b *genericBroadcaster) addListenerInternal() interface{} {
	if b.channelType == nil || b.receiveChannelType == nil {
		return nil
	}
	sendCh := reflect.MakeChan(b.channelType, subscriberChannelBufferLength)
	receiveCh := sendCh.Convert(b.receiveChannelType).Interface()
	chPair := genericChannelPair{sendCh: sendCh, receiveCh: receiveCh}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.subscribers = append(b.subscribers, chPair)
	return receiveCh
}

// Reflective implementation of RemoveListener. The ch parameter is really of type <-chan X.
func (b *genericBroadcaster) removeListenerInternal(ch interface{}) {
	b.lock.Lock()
	defer b.lock.Unlock()
	ss := b.subscribers
	for i, s := range ss {
		// The following equality test is the reason why we have to store both the sendCh (chan X) and
		// the receiveCh (<-chan X) for each subscriber; "s.sendCh == ch" would not be true because
		// they're of two different types.
		if s.receiveCh == ch {
			copy(ss[i:], ss[i+1:])
			ss[len(ss)-1] = genericChannelPair{}
			b.subscribers = ss[:len(ss)-1]
			s.sendCh.Close() // this is reflect.Value.Close(), which works for any channel
			break
		}
	}
}

func (b *genericBroadcaster) hasListeners() bool {
	return len(b.subscribers) > 0
}

// Reflective implementation of Broadcast. The value parameter must be of type X.
func (b *genericBroadcaster) broadcastInternal(value interface{}) {
	if reflect.TypeOf(value) != b.elementType {
		return // sending a value of the wrong type with reflect.Value.Send would cause a panic
	}
	var ss []genericChannelPair
	b.lock.Lock()
	if len(b.subscribers) > 0 {
		ss = make([]genericChannelPair, len(b.subscribers))
		copy(ss, b.subscribers)
	}
	b.lock.Unlock()
	if len(ss) > 0 {
		genericValue := reflect.ValueOf(value)
		for _, ch := range ss {
			ch.sendCh.Send(genericValue) // this is reflect.Value.Send(), which works for any channel
		}
	}
}

func (b *genericBroadcaster) close() {
	b.lock.Lock()
	defer b.lock.Unlock()
	for _, s := range b.subscribers {
		s.sendCh.Close()
	}
	b.subscribers = nil
}

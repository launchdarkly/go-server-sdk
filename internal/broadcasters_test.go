package internal

import (
	"reflect"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataStoreStatusBroadcaster(t *testing.T) {
	testBroadcasterGenerically(t, NewDataStoreStatusBroadcaster,
		func() interface{} { return interfaces.DataStoreStatus{} })
}

func TestDataSourceStatusBroadcaster(t *testing.T) {
	testBroadcasterGenerically(t, NewDataSourceStatusBroadcaster,
		func() interface{} { return interfaces.DataSourceStatus{State: interfaces.DataSourceStateValid} })
}

func TestFlagChangeEventBroadcaster(t *testing.T) {
	testBroadcasterGenerically(t, NewFlagChangeEventBroadcaster,
		func() interface{} { return interfaces.FlagChangeEvent{Key: "flag"} })
}

func TestBigSegmentStoreStatusBroadcaster(t *testing.T) {
	testBroadcasterGenerically(t, NewBigSegmentStoreStatusBroadcaster,
		func() interface{} { return interfaces.BigSegmentStoreStatus{Available: true} })
}

// Runs a standard test suite that should work for any of our broadcaster types.
//
// broadcasterFactory: a constructor function (New___Broadcaster)
// valueFactory: a function that returns an instance of the value/event type that is sent by this broadcaster
//
// The test requires that the broadcaster type have the standard AddListener, RemoveListener, Broadcast, and
// Close methods. It may also have a HasListeners method (we don't need this for all broadcasters). It calls
// these methods via reflection to verify that publishing and subscribing work correctly.
//
// The test will fail if any of the required methods are missing, and panic if they're implemented with the
// wrong parameter types or return types.
func testBroadcasterGenerically(t *testing.T, broadcasterFactory interface{}, valueFactory func() interface{}) {
	factoryValue := reflect.ValueOf(broadcasterFactory)
	require.Equal(t, reflect.Func, factoryValue.Type().Kind())
	require.Equal(t, 0, factoryValue.Type().NumIn())
	require.Equal(t, 1, factoryValue.Type().NumOut())

	withBroadcaster := func(t *testing.T, action func(broadcasterMethods)) {
		b := getBroadcasterMethods(t, factoryValue.Call(nil)[0].Interface())
		defer b.close()
		action(b)
	}

	t.Run("broadcast with no subscribers", func(t *testing.T) {
		withBroadcaster(t, func(b broadcasterMethods) {
			b.broadcast(valueFactory())
		})
	})

	t.Run("broadcast with subscribers", func(t *testing.T) {
		withBroadcaster(t, func(b broadcasterMethods) {
			ch1 := b.addListener()
			ch2 := b.addListener()

			value := valueFactory()
			b.broadcast(value)

			assert.Equal(t, value, ch1.receive(t))
			assert.Equal(t, value, ch2.receive(t))
		})
	})

	t.Run("unregister subscriber", func(t *testing.T) {
		withBroadcaster(t, func(b broadcasterMethods) {
			ch1 := b.addListener()
			ch2 := b.addListener()
			b.removeListener(ch1)

			value := valueFactory()
			b.broadcast(value)

			assert.Equal(t, 0, ch1.len())
			assert.Equal(t, value, ch2.receive(t))
		})
	})

	t.Run("hasListeners", func(t *testing.T) {
		withBroadcaster(t, func(b broadcasterMethods) {
			if b.hasListeners == nil {
				t.SkipNow()
			}

			assert.False(t, b.hasListeners())

			ch1 := b.addListener()
			ch2 := b.addListener()

			assert.True(t, b.hasListeners())

			b.removeListener(ch1)

			assert.True(t, b.hasListeners())

			b.removeListener(ch2)

			assert.False(t, b.hasListeners())
		})
	})

}

type genericChannel struct {
	ch reflect.Value
}

func (g genericChannel) receive(t *testing.T) interface{} {
	value, ok := g.ch.Recv()
	require.True(t, ok)
	return value.Interface()
}

func (g genericChannel) len() int {
	return g.ch.Len()
}

type broadcasterMethods struct {
	addListener    func() genericChannel
	removeListener func(genericChannel)
	hasListeners   func() bool
	broadcast      func(interface{})
	close          func()
}

func getBroadcasterMethods(t *testing.T, b interface{}) broadcasterMethods {
	var ret broadcasterMethods

	bv := reflect.ValueOf(b)
	bt := reflect.TypeOf(b)
	require.Equal(t, reflect.Ptr, bt.Kind())

	requireMethod := func(name string, paramCount int, returnCount int) reflect.Method {
		m, ok := bt.MethodByName(name)
		require.True(t, ok, "type %s has no %s method", bt, name)
		require.Equal(t, paramCount+1, m.Type.NumIn())
		require.Equal(t, returnCount, m.Type.NumOut())
		return m
	}

	closeMethod := requireMethod("Close", 0, 0)
	ret.close = func() {
		closeMethod.Func.Call([]reflect.Value{bv})
	}

	addListenerMethod := requireMethod("AddListener", 0, 1)
	ret.addListener = func() genericChannel {
		return genericChannel{ch: addListenerMethod.Func.Call([]reflect.Value{bv})[0]}
	}

	removeListenerMethod := requireMethod("RemoveListener", 1, 0)
	ret.removeListener = func(ch genericChannel) {
		removeListenerMethod.Func.Call([]reflect.Value{bv, ch.ch})
	}

	if _, ok := bt.MethodByName("HasListeners"); ok {
		hasListenersMethod := requireMethod("HasListeners", 0, 1)
		ret.hasListeners = func() bool {
			return hasListenersMethod.Func.Call([]reflect.Value{bv})[0].Bool()
		}
	}

	broadcastMethod := requireMethod("Broadcast", 1, 0)
	ret.broadcast = func(value interface{}) {
		broadcastMethod.Func.Call([]reflect.Value{bv, reflect.ValueOf(value)})
	}

	return ret
}

func TestGenericBroadcasterValidation(t *testing.T) {
	// This verifies that genericBroadcaster won't panic if we pass invalid types to it.

	t.Run("value is wrong type", func(t *testing.T) {
		b := newGenericBroadcaster((chan int)(nil), (<-chan int)(nil))
		b.broadcastInternal("this isn't an int")
	})

	t.Run("sendCh and receiveCh have non-matching types", func(t *testing.T) {
		b := newGenericBroadcaster((chan bool)(nil), (<-chan int)(nil))
		assert.Nil(t, b.addListenerInternal())
		b.broadcastInternal(1)
	})

	t.Run("sendCh is of a send-only channel type", func(t *testing.T) {
		b := newGenericBroadcaster((chan<- int)(nil), (<-chan int)(nil))
		assert.Nil(t, b.addListenerInternal())
		b.broadcastInternal(1)
	})

	t.Run("sendCh is of a receive-only channel type", func(t *testing.T) {
		b := newGenericBroadcaster((<-chan int)(nil), (<-chan int)(nil))
		assert.Nil(t, b.addListenerInternal())
		b.broadcastInternal(1)
	})

	t.Run("receiveCh is of a send-only channel type", func(t *testing.T) {
		b := newGenericBroadcaster((chan int)(nil), (chan<- int)(nil))
		assert.Nil(t, b.addListenerInternal())
		b.broadcastInternal(1)
	})

	t.Run("sendCh is of a bidirectional channel type", func(t *testing.T) {
		b := newGenericBroadcaster((chan int)(nil), (chan int)(nil))
		assert.Nil(t, b.addListenerInternal())
		b.broadcastInternal(1)
	})
}

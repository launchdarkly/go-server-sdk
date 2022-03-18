package internal

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// flagTrackerImpl is the internal implementation of FlagTracker. It's not exported because
// the rest of the SDK code only interacts with the public interface.
//
// The underlying FlagChangeEventBroadcaster receives notifications of flag changes in general.
// When a value change listener is created with AddFlagValueChangeListener, this is implemented
// by creating a regular FlagChangeEvent channel and starting a goroutine that reads it and posts
// events as appropriate to a FlagValueChangeEvent channel; the flagTrackerImpl maintains its own
// mapping of this to the underlying channel which is necessary for unregistering it.
type flagTrackerImpl struct {
	broadcaster              *FlagChangeEventBroadcaster
	evaluateFn               func(string, ldcontext.Context, ldvalue.Value) ldvalue.Value
	valueChangeSubscriptions map[<-chan interfaces.FlagValueChangeEvent]<-chan interfaces.FlagChangeEvent
	lock                     sync.Mutex
}

// NewFlagTrackerImpl creates the internal implementation of FlagTracker.
func NewFlagTrackerImpl(
	broadcaster *FlagChangeEventBroadcaster,
	evaluateFn func(flagKey string, context ldcontext.Context, defaultValue ldvalue.Value) ldvalue.Value,
) interfaces.FlagTracker {
	return &flagTrackerImpl{
		broadcaster:              broadcaster,
		evaluateFn:               evaluateFn,
		valueChangeSubscriptions: make(map[<-chan interfaces.FlagValueChangeEvent]<-chan interfaces.FlagChangeEvent),
	}
}

// AddFlagChangeListener is a standard method of FlagTracker.
func (f *flagTrackerImpl) AddFlagChangeListener() <-chan interfaces.FlagChangeEvent {
	return f.broadcaster.AddListener()
}

// RemoveFlagChangeListener is a standard method of FlagTracker.
func (f *flagTrackerImpl) RemoveFlagChangeListener(listener <-chan interfaces.FlagChangeEvent) {
	f.broadcaster.RemoveListener(listener)
}

// AddFlagValueChangeListener is a standard method of FlagTracker.
func (f *flagTrackerImpl) AddFlagValueChangeListener(
	flagKey string,
	context ldcontext.Context,
	defaultValue ldvalue.Value,
) <-chan interfaces.FlagValueChangeEvent {
	valueCh := make(chan interfaces.FlagValueChangeEvent, subscriberChannelBufferLength)
	flagCh := f.broadcaster.AddListener()
	go runValueChangeListener(flagCh, valueCh, f.evaluateFn, flagKey, context, defaultValue)

	f.lock.Lock()
	f.valueChangeSubscriptions[valueCh] = flagCh
	f.lock.Unlock()

	return valueCh
}

// RemoveFlagValueChangeListener is a standard method of FlagTracker.
func (f *flagTrackerImpl) RemoveFlagValueChangeListener(listener <-chan interfaces.FlagValueChangeEvent) {
	f.lock.Lock()
	flagCh, ok := f.valueChangeSubscriptions[listener]
	delete(f.valueChangeSubscriptions, listener)
	f.lock.Unlock()

	if ok {
		f.broadcaster.RemoveListener(flagCh)
	}
}

func runValueChangeListener(
	flagCh <-chan interfaces.FlagChangeEvent,
	valueCh chan<- interfaces.FlagValueChangeEvent,
	evaluateFn func(flagKey string, context ldcontext.Context, defaultValue ldvalue.Value) ldvalue.Value,
	flagKey string,
	context ldcontext.Context,
	defaultValue ldvalue.Value,
) {
	currentValue := evaluateFn(flagKey, context, defaultValue)
	for {
		flagChange, ok := <-flagCh
		if !ok {
			// the underlying subscription has been unregistered
			close(valueCh)
			return
		}
		if flagChange.Key != flagKey {
			continue
		}
		newValue := evaluateFn(flagKey, context, defaultValue)
		if newValue.Equal(currentValue) {
			continue
		}
		event := interfaces.FlagValueChangeEvent{Key: flagKey, OldValue: currentValue, NewValue: newValue}
		currentValue = newValue
		valueCh <- event
	}
}

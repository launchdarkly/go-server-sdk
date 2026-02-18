package main

import (
	"context"
	"sync"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/testservice/servicedef"
)

// listenerEntry holds the cancellation handle for one registered listener goroutine.
type listenerEntry struct {
	cancel context.CancelFunc
}

// listenerRegistry manages all active flag change listener registrations for a single
// SDK client entity. It is safe to use from multiple goroutines.
type listenerRegistry struct {
	mu      sync.Mutex
	entries map[string]*listenerEntry // keyed by listenerId
	tracker interfaces.FlagTracker
}

func newListenerRegistry(tracker interfaces.FlagTracker) *listenerRegistry {
	return &listenerRegistry{
		entries: make(map[string]*listenerEntry),
		tracker: tracker,
	}
}

// registerFlagChangeListener subscribes to general flag configuration changes. If flagKey
// is non-empty, only events for that specific flag are forwarded to the callback URI.
func (r *listenerRegistry) registerFlagChangeListener(listenerID, flagKey, callbackURI string) {
	ch := r.tracker.AddFlagChangeListener()
	ctx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.entries[listenerID] = &listenerEntry{cancel: cancel}
	r.mu.Unlock()

	svc := callbackService{baseURL: callbackURI}
	go func() {
		defer r.tracker.RemoveFlagChangeListener(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				if flagKey != "" && event.Key != flagKey {
					continue
				}
				_ = svc.post("", servicedef.ListenerNotification{
					ListenerID: listenerID,
					FlagKey:    event.Key,
				}, nil)
			}
		}
	}()
}

// registerFlagValueChangeListener subscribes to value changes for a specific flag and
// evaluation context. The callback is invoked only when the evaluated value actually
// changes; configuration changes that leave the value unchanged are suppressed by the SDK.
func (r *listenerRegistry) registerFlagValueChangeListener(
	listenerID, flagKey string,
	evalCtx ldcontext.Context,
	defaultValue ldvalue.Value,
	callbackURI string,
) {
	ch := r.tracker.AddFlagValueChangeListener(flagKey, evalCtx, defaultValue)
	ctx, cancel := context.WithCancel(context.Background())

	r.mu.Lock()
	r.entries[listenerID] = &listenerEntry{cancel: cancel}
	r.mu.Unlock()

	svc := callbackService{baseURL: callbackURI}
	go func() {
		defer r.tracker.RemoveFlagValueChangeListener(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				oldVal := event.OldValue
				newVal := event.NewValue
				_ = svc.post("", servicedef.ListenerNotification{
					ListenerID: listenerID,
					FlagKey:    event.Key,
					OldValue:   &oldVal,
					NewValue:   &newVal,
				}, nil)
			}
		}
	}()
}

// unregister stops the listener goroutine for the given ID and removes it from the
// registry. Returns false if no listener with that ID was found.
func (r *listenerRegistry) unregister(listenerID string) bool {
	r.mu.Lock()
	entry, ok := r.entries[listenerID]
	if ok {
		delete(r.entries, listenerID)
	}
	r.mu.Unlock()

	if ok {
		entry.cancel()
	}
	return ok
}

// closeAll stops all active listener goroutines. Called when the SDK client entity closes.
func (r *listenerRegistry) closeAll() {
	r.mu.Lock()
	entries := r.entries
	r.entries = make(map[string]*listenerEntry)
	r.mu.Unlock()

	for _, entry := range entries {
		entry.cancel()
	}
}

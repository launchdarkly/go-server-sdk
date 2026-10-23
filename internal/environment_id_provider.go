package internal

import (
	"sync/atomic"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// EnvironmentIDProvider describes the interface for an object that can provide an EnvironmentID.
type EnvironmentIDProvider interface {
	// GetEnvironmentID gets the provided EnvironmentID as an optional string.
	GetEnvironmentID() ldvalue.OptionalString
}

// EnvironmentIDHolder stores an EnvironmentID that a data source sets while other goroutines read it.
//
// The hook runner reads the EnvironmentID on every evaluation and track call, so reads must not take
// a lock. The zero value holds an undefined EnvironmentID.
type EnvironmentIDHolder struct {
	value atomic.Pointer[ldvalue.OptionalString]
}

// Get returns the stored EnvironmentID.
func (h *EnvironmentIDHolder) Get() ldvalue.OptionalString {
	if value := h.value.Load(); value != nil {
		return *value
	}
	return ldvalue.OptionalString{}
}

// Set stores an EnvironmentID. Data sources set the same value on every update, so Set does not
// allocate when the value is unchanged.
func (h *EnvironmentIDHolder) Set(environmentID ldvalue.OptionalString) {
	if current := h.value.Load(); current != nil && *current == environmentID {
		return
	}
	// Copying keeps the parameter on the stack, so only a changed value escapes to the heap.
	value := environmentID
	h.value.Store(&value)
}

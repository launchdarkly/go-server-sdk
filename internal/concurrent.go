package internal

import (
	"sync/atomic"
)

// AtomicBoolean is a simple atomic boolean type based on sync/atomic. Since sync/atomic supports
// only integer types, the implementation uses an int32. (Note: we should be able to get rid of
// this once our minimum Go version becomes 1.19 or higher.)
type AtomicBoolean struct {
	value int32
}

// Get returns the current value.
func (a *AtomicBoolean) Get() bool {
	return int32ToBoolean(atomic.LoadInt32(&a.value))
}

// Set updates the value.
func (a *AtomicBoolean) Set(value bool) {
	atomic.StoreInt32(&a.value, booleanToInt32(value))
}

// GetAndSet atomically updates the value and returns the previous value.
func (a *AtomicBoolean) GetAndSet(value bool) bool {
	return int32ToBoolean(atomic.SwapInt32(&a.value, booleanToInt32(value)))
}

func booleanToInt32(value bool) int32 {
	if value {
		return 1
	}
	return 0
}

func int32ToBoolean(value int32) bool {
	return value != 0
}

package sharedtest

import (
	"testing"
	"time"
)

// RequireValue returns the next value from the channel, or forces an immediate test failure
// and exit if the timeout expires first.
func RequireValue[V any](t *testing.T, ch <-chan V, timeout time.Duration) V {
	select {
	case v := <-ch:
		return v
	case <-time.After(timeout):
		var empty V
		t.Errorf("expected a %T value from channel but did not receive one in %s", empty, timeout)
		t.FailNow()
		return empty // never reached
	}
}

// AssertNoMoreValues asserts that no value is available from the channel within the timeout.
func AssertNoMoreValues[V any](t *testing.T, ch <-chan V, timeout time.Duration) bool {
	select {
	case v, ok := <-ch:
		if ok {
			t.Errorf("expected no more %T values from channel but got one: %+v", v, v)
			return false
		}
		return true
	case <-time.After(timeout):
		return true
	}
}

// RequireNoMoreValues is equivalent to AssertNoMoreValues except that it forces an immediate
// test exit on failure.
func RequireNoMoreValues[V any](t *testing.T, ch <-chan V, timeout time.Duration) {
	if !AssertNoMoreValues(t, ch, timeout) {
		t.FailNow()
	}
}

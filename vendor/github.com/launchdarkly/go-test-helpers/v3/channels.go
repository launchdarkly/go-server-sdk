package helpers

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TryReceive waits for a value from the channel and returns (value, true, false) if
// successful; (<empty>, false, false) if the timeout expired first; or
// (<empty>, false, true) if the channel was closed.
func TryReceive[V any](ch <-chan V, timeout time.Duration) (V, bool, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	select {
	case v, ok := <-ch:
		if ok {
			return v, true, false
		}
		return v, false, true
	case <-deadline.C:
		var empty V
		return empty, false, false
	}
}

// RequireValue returns the next value from the channel, or forces an immediate test failure
// and exit if the timeout expires first.
func RequireValue[V any](t require.TestingT, ch <-chan V, timeout time.Duration, customMessageAndArgs ...any) V {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	v, ok, closed := TryReceive(ch, timeout)
	if ok {
		return v
	}
	var empty V
	if closed {
		failWithMessageAndArgs(t, customMessageAndArgs,
			"expected a %T value from channel but the channel was closed", empty)
	} else {
		failWithMessageAndArgs(t, customMessageAndArgs,
			"expected a %T value from channel but did not receive one in %s", empty, timeout)
	}
	t.FailNow()
	return empty // never reached
}

// AssertNoMoreValues asserts that no value is available from the channel within the timeout,
// but that the channel was not closed.
func AssertNoMoreValues[V any](
	t assert.TestingT,
	ch <-chan V,
	timeout time.Duration,
	customMessageAndArgs ...any,
) bool {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	v, ok, closed := TryReceive(ch, timeout)
	if ok {
		failWithMessageAndArgs(t, customMessageAndArgs,
			"expected no more %T values from channel but got one: %+v", v, v)
		return false
	}
	if closed {
		failWithMessageAndArgs(t, customMessageAndArgs, "channel was unexpectedly closed")
		return false
	}
	return true
}

// AssertChannelClosed asserts that the channel is closed within the timeout, sending no values.
func AssertChannelClosed[V any](
	t assert.TestingT,
	ch <-chan V,
	timeout time.Duration,
	customMessageAndArgs ...any,
) bool {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	v, ok, closed := TryReceive(ch, timeout)
	if ok {
		failWithMessageAndArgs(t, customMessageAndArgs,
			"expected no more %T values from channel but got one: %+v", v, v)
		return false
	}
	if !closed {
		failWithMessageAndArgs(t, customMessageAndArgs,
			"expected channel to be closed within %s but it was not", timeout)
		return false
	}
	return true
}

// AssertChannelNotClosed asserts that the channel is not closed within the timeout, consuming
// any values that may be sent during that time.
func AssertChannelNotClosed[V any](
	t assert.TestingT,
	ch <-chan V,
	timeout time.Duration,
	customMessageAndArgs ...any,
) bool {
	if t, ok := t.(interface{ Helper() }); ok {
		t.Helper()
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				failWithMessageAndArgs(t, customMessageAndArgs, "channel was unexpectedly closed")
				return false
			}
		case <-deadline.C:
			return true
		}
	}
}

func failWithMessageAndArgs(t assert.TestingT, customMessageAndArgs []any, defaultMsg string, defaultArgs ...any) {
	t.Errorf(defaultMsg, defaultArgs...)
	if len(customMessageAndArgs) != 0 {
		t.Errorf(fmt.Sprintf("%s", customMessageAndArgs[0]), customMessageAndArgs[1:]...)
	}
}

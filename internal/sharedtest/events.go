package sharedtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
)

// ExpectFlagChangeEvents asserts that a channel receives flag change events for the specified keys (in
// any order) and then does not receive any more events for the next 100ms.
func ExpectFlagChangeEvents(t *testing.T, ch <-chan interfaces.FlagChangeEvent, keys ...string) {
	expectedChangedFlagKeys := make(map[string]bool)
	for _, key := range keys {
		expectedChangedFlagKeys[key] = true
	}
	actualChangedFlagKeys := make(map[string]bool)
ReadLoop:
	for i := 0; i < len(keys); i++ {
		select {
		case event, ok := <-ch:
			if !ok {
				break ReadLoop
			}
			actualChangedFlagKeys[event.Key] = true
		case <-time.After(time.Second):
			assert.Fail(t, "did not receive expected event", "expected: %v, received: %v",
				expectedChangedFlagKeys, actualChangedFlagKeys)
			return
		}
	}
	assert.Equal(t, expectedChangedFlagKeys, actualChangedFlagKeys)
	AssertNoMoreValues(t, ch, time.Millisecond*100)
}

package internal

import (
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	intf "github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
)

func TestFlagChangeListeners(t *testing.T) {
	flagKey := "flagkey"

	broadcaster := NewBroadcaster[interfaces.FlagChangeEvent]()
	defer broadcaster.Close()
	tracker := NewFlagTrackerImpl(broadcaster, nil)

	ch1 := tracker.AddFlagChangeListener()
	ch2 := tracker.AddFlagChangeListener()

	broadcaster.Broadcast(intf.FlagChangeEvent{Key: flagKey})

	sharedtest.ExpectFlagChangeEvents(t, ch1, flagKey)
	sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)

	tracker.RemoveFlagChangeListener(ch1)

	broadcaster.Broadcast(intf.FlagChangeEvent{Key: flagKey})

	sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)
}

func TestFlagValueChangeListener(t *testing.T) {
	flagKey := "important-flag"
	user := lduser.NewUser("important-user")
	otherUser := lduser.NewUser("unimportant-user")
	resultMap := make(map[string]ldvalue.Value)
	resultLock := sync.Mutex{}
	timeout := time.Millisecond * 100

	broadcaster := NewBroadcaster[interfaces.FlagChangeEvent]()
	defer broadcaster.Close()
	tracker := NewFlagTrackerImpl(broadcaster, func(flag string, user ldcontext.Context, defaultValue ldvalue.Value) ldvalue.Value {
		resultLock.Lock()
		defer resultLock.Unlock()
		return resultMap[user.Key()]
	})

	resultMap[user.Key()] = ldvalue.Bool(false)
	resultMap[otherUser.Key()] = ldvalue.Bool(false)

	ch1 := tracker.AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
	ch2 := tracker.AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
	ch3 := tracker.AddFlagValueChangeListener(flagKey, otherUser, ldvalue.Null())

	tracker.RemoveFlagValueChangeListener(ch2) // just verifying that the remove method works
	th.AssertChannelClosed(t, ch2, time.Millisecond)

	th.AssertNoMoreValues(t, ch1, timeout)
	th.AssertNoMoreValues(t, ch3, timeout)

	// make the flag true for the first user only, and broadcast a flag change event
	resultLock.Lock()
	resultMap[user.Key()] = ldvalue.Bool(true)
	resultLock.Unlock()
	broadcaster.Broadcast(intf.FlagChangeEvent{Key: flagKey})

	// ch1 receives a value change event
	event1 := <-ch1
	assert.Equal(t, flagKey, event1.Key)
	assert.Equal(t, ldvalue.Bool(false), event1.OldValue)
	assert.Equal(t, ldvalue.Bool(true), event1.NewValue)

	// ch3 doesn't receive one, because the flag's value hasn't changed for otherUser
	th.AssertNoMoreValues(t, ch3, timeout)

	// broadcast a flag change event for a different flag
	broadcaster.Broadcast(intf.FlagChangeEvent{Key: "other-flag"})
	th.AssertNoMoreValues(t, ch1, timeout)
}
